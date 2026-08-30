use std::{ffi::OsStr, io, os::windows::ffi::OsStrExt as _, path::Path};
use windows::{
    Win32::{
        Foundation::{CloseHandle, HANDLE, HLOCAL, LocalFree, WIN32_ERROR},
        Security::{
            ACL,
            Authorization::{
                EXPLICIT_ACCESS_W, GRANT_ACCESS, GetExplicitEntriesFromAclW, GetNamedSecurityInfoW,
                SE_FILE_OBJECT, SetEntriesInAclW, SetNamedSecurityInfoW, TRUSTEE_IS_SID,
                TRUSTEE_IS_USER, TRUSTEE_W,
            },
            DACL_SECURITY_INFORMATION, EqualSid, GetTokenInformation, NO_INHERITANCE,
            OWNER_SECURITY_INFORMATION, PROTECTED_DACL_SECURITY_INFORMATION, PSECURITY_DESCRIPTOR,
            PSID, SUB_CONTAINERS_AND_OBJECTS_INHERIT, TOKEN_QUERY, TOKEN_USER, TokenUser,
        },
        Storage::FileSystem::FILE_ALL_ACCESS,
        System::Threading::{GetCurrentProcess, OpenProcessToken},
    },
    core::{PCWSTR, PWSTR},
};

struct UserSid {
    _storage: Vec<usize>,
    value: PSID,
}

struct LocalAllocation(HLOCAL);

impl Drop for LocalAllocation {
    fn drop(&mut self) {
        unsafe { LocalFree(Some(self.0)) };
    }
}

pub fn protect_private_path(path: &Path, directory: bool) -> io::Result<()> {
    let sid = current_user_sid()?;
    let inheritance = if directory {
        SUB_CONTAINERS_AND_OBJECTS_INHERIT
    } else {
        NO_INHERITANCE
    };
    let entry = EXPLICIT_ACCESS_W {
        grfAccessPermissions: FILE_ALL_ACCESS.0,
        grfAccessMode: GRANT_ACCESS,
        grfInheritance: inheritance,
        Trustee: TRUSTEE_W {
            TrusteeForm: TRUSTEE_IS_SID,
            TrusteeType: TRUSTEE_IS_USER,
            ptstrName: PWSTR(sid.value.0.cast()),
            ..Default::default()
        },
    };
    let mut acl = std::ptr::null_mut::<ACL>();
    check(unsafe { SetEntriesInAclW(Some(&[entry]), None, &mut acl) })?;
    let allocation = LocalAllocation(HLOCAL(acl.cast()));
    let mut path_wide = wide(path.as_os_str());
    check(unsafe {
        SetNamedSecurityInfoW(
            PWSTR(path_wide.as_mut_ptr()),
            SE_FILE_OBJECT,
            DACL_SECURITY_INFORMATION
                | OWNER_SECURITY_INFORMATION
                | PROTECTED_DACL_SECURITY_INFORMATION,
            Some(sid.value),
            None,
            Some(acl),
            None,
        )
    })?;
    drop(allocation);
    validate_private_path(path, directory)
}

pub fn validate_private_path(path: &Path, directory: bool) -> io::Result<()> {
    let metadata = std::fs::symlink_metadata(path)?;
    if metadata.file_type().is_symlink()
        || (directory && !metadata.is_dir())
        || (!directory && !metadata.is_file())
    {
        return Err(unsafe_acl());
    }
    let sid = current_user_sid()?;
    let mut owner = PSID::default();
    let mut acl = std::ptr::null_mut::<ACL>();
    let mut descriptor = PSECURITY_DESCRIPTOR::default();
    let path = wide(path.as_os_str());
    check(unsafe {
        GetNamedSecurityInfoW(
            PCWSTR(path.as_ptr()),
            SE_FILE_OBJECT,
            OWNER_SECURITY_INFORMATION | DACL_SECURITY_INFORMATION,
            Some(&mut owner),
            None,
            Some(&mut acl),
            None,
            &mut descriptor,
        )
    })?;
    let descriptor = LocalAllocation(HLOCAL(descriptor.0));
    if owner.is_invalid() || acl.is_null() || unsafe { EqualSid(owner, sid.value) }.is_err() {
        return Err(unsafe_acl());
    }
    validate_entries(acl, sid.value, directory)?;
    drop(descriptor);
    Ok(())
}

fn validate_entries(acl: *mut ACL, sid: PSID, directory: bool) -> io::Result<()> {
    let mut count = 0_u32;
    let mut entries = std::ptr::null_mut::<EXPLICIT_ACCESS_W>();
    check(unsafe { GetExplicitEntriesFromAclW(acl, &mut count, &mut entries) })?;
    let allocation = LocalAllocation(HLOCAL(entries.cast()));
    if count == 0 || entries.is_null() {
        return Err(unsafe_acl());
    }
    let values = unsafe { std::slice::from_raw_parts(entries, count as usize) };
    let inheritance = if directory {
        SUB_CONTAINERS_AND_OBJECTS_INHERIT
    } else {
        NO_INHERITANCE
    };
    let safe = values.len() == 1
        && values[0].grfAccessMode == GRANT_ACCESS
        && values[0].grfAccessPermissions & FILE_ALL_ACCESS.0 == FILE_ALL_ACCESS.0
        && values[0].grfInheritance == inheritance
        && values[0].Trustee.TrusteeForm == TRUSTEE_IS_SID
        && unsafe { EqualSid(PSID(values[0].Trustee.ptstrName.0.cast()), sid) }.is_ok();
    drop(allocation);
    safe.then_some(()).ok_or_else(unsafe_acl)
}

fn current_user_sid() -> io::Result<UserSid> {
    let mut token = HANDLE::default();
    unsafe { OpenProcessToken(GetCurrentProcess(), TOKEN_QUERY, &mut token) }
        .map_err(windows_io)?;
    let mut bytes = 0_u32;
    let _ = unsafe { GetTokenInformation(token, TokenUser, None, 0, &mut bytes) };
    let words = (bytes as usize).div_ceil(std::mem::size_of::<usize>());
    let mut storage = vec![0_usize; words];
    let result = unsafe {
        GetTokenInformation(
            token,
            TokenUser,
            Some(storage.as_mut_ptr().cast()),
            bytes,
            &mut bytes,
        )
    };
    unsafe { CloseHandle(token) }.map_err(windows_io)?;
    result.map_err(windows_io)?;
    let user = unsafe { &*storage.as_ptr().cast::<TOKEN_USER>() };
    Ok(UserSid {
        value: user.User.Sid,
        _storage: storage,
    })
}

fn wide(value: &OsStr) -> Vec<u16> {
    value.encode_wide().chain(Some(0)).collect()
}

fn check(error: WIN32_ERROR) -> io::Result<()> {
    if error.0 == 0 {
        Ok(())
    } else {
        Err(io::Error::from_raw_os_error(error.0 as i32))
    }
}

fn windows_io(error: windows::core::Error) -> io::Error {
    io::Error::from_raw_os_error(error.code().0)
}

fn unsafe_acl() -> io::Error {
    io::Error::new(
        io::ErrorKind::PermissionDenied,
        "RC private path has an unsafe Windows ACL or type",
    )
}

#[cfg(test)]
mod tests {
    use super::*;

    fn fixture() -> std::path::PathBuf {
        std::env::temp_dir().join(format!(
            "rc-acl-{}-{}",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap()
                .as_nanos()
        ))
    }

    #[test]
    fn private_file_and_directory_acl_validate() {
        let directory = fixture();
        std::fs::create_dir(&directory).unwrap();
        protect_private_path(&directory, true).unwrap();
        validate_private_path(&directory, true).unwrap();
        let file = directory.join("secret");
        std::fs::write(&file, b"secret").unwrap();
        protect_private_path(&file, false).unwrap();
        validate_private_path(&file, false).unwrap();
        std::fs::remove_dir_all(directory).unwrap();
    }

    #[test]
    fn null_dacl_is_rejected_as_broad() {
        let directory = fixture();
        std::fs::create_dir(&directory).unwrap();
        protect_private_path(&directory, true).unwrap();
        let file = directory.join("secret");
        std::fs::write(&file, b"secret").unwrap();
        protect_private_path(&file, false).unwrap();
        let mut path = wide(file.as_os_str());
        check(unsafe {
            SetNamedSecurityInfoW(
                PWSTR(path.as_mut_ptr()),
                SE_FILE_OBJECT,
                DACL_SECURITY_INFORMATION,
                None,
                None,
                None,
                None,
            )
        })
        .unwrap();
        assert_eq!(
            validate_private_path(&file, false).unwrap_err().kind(),
            io::ErrorKind::PermissionDenied
        );
        protect_private_path(&file, false).unwrap();
        std::fs::remove_dir_all(directory).unwrap();
    }

    #[test]
    fn empty_dacl_is_rejected_without_reading_a_null_entry_array() {
        use windows::Win32::Security::{ACL_REVISION, InitializeAcl};

        let directory = fixture();
        std::fs::create_dir(&directory).unwrap();
        protect_private_path(&directory, true).unwrap();
        let file = directory.join("secret");
        std::fs::write(&file, b"secret").unwrap();
        protect_private_path(&file, false).unwrap();
        let mut acl = ACL::default();
        unsafe { InitializeAcl(&mut acl, std::mem::size_of::<ACL>() as u32, ACL_REVISION) }
            .unwrap();
        let mut path = wide(file.as_os_str());
        check(unsafe {
            SetNamedSecurityInfoW(
                PWSTR(path.as_mut_ptr()),
                SE_FILE_OBJECT,
                DACL_SECURITY_INFORMATION | PROTECTED_DACL_SECURITY_INFORMATION,
                None,
                None,
                Some(&acl),
                None,
            )
        })
        .unwrap();
        assert_eq!(
            validate_private_path(&file, false).unwrap_err().kind(),
            io::ErrorKind::PermissionDenied
        );
        protect_private_path(&file, false).unwrap();
        std::fs::remove_dir_all(directory).unwrap();
    }
}
