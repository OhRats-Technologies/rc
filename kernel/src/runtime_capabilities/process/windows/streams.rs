use super::*;

fn powershell(source: &str) -> SpawnRequest {
    SpawnRequest {
        program: "powershell.exe".into(),
        args: [
            "-NoLogo",
            "-NoProfile",
            "-NonInteractive",
            "-Command",
            source,
        ]
        .into_iter()
        .map(str::to_owned)
        .collect(),
        cwd: None,
        environment: Environment {
            base: EnvironmentBase::Inherit,
            changes: Vec::new(),
        },
        terminal: None,
    }
}

#[test]
fn piped_stdout_and_stderr_preserve_arbitrary_bytes() {
    let source = concat!(
        "$o=[Console]::OpenStandardOutput();$e=[Console]::OpenStandardError();",
        "$a=[byte[]](0,255,10);$b=[byte[]](254,1,13);",
        "$o.Write($a,0,$a.Length);$e.Write($b,0,$b.Length)"
    );
    let mut group = Group::new().unwrap();
    let spawned = spawn(&mut group, powershell(source)).unwrap();
    assert_eq!(wait(&mut group, spawned.native_child).code, Some(0));
    let StreamValue::Reader(mut stdout) = spawned.stdout else {
        panic!("stdout is not readable")
    };
    let StreamValue::Reader(mut stderr) = spawned.stderr.unwrap() else {
        panic!("stderr is not readable")
    };
    let mut out = Vec::new();
    let mut err = Vec::new();
    stdout.read_to_end(&mut out).unwrap();
    stderr.read_to_end(&mut err).unwrap();
    assert_eq!(out, [0, 255, 10]);
    assert_eq!(err, [254, 1, 13]);
}

#[test]
fn piped_stdin_preserves_bytes_and_eof() {
    let source = concat!(
        "$i=[Console]::OpenStandardInput();",
        "$o=[Console]::OpenStandardOutput();$i.CopyTo($o)"
    );
    let mut group = Group::new().unwrap();
    let mut spawned = spawn(&mut group, powershell(source)).unwrap();
    let StreamValue::Writer(mut stdin) = spawned.stdin.take().unwrap() else {
        panic!("stdin is not writable")
    };
    let expected = [0, 1, 255, 10, 13];
    stdin.write_all(&expected).unwrap();
    drop(stdin);
    assert_eq!(wait(&mut group, spawned.native_child).code, Some(0));
    let StreamValue::Reader(mut stdout) = spawned.stdout else {
        panic!("stdout is not readable")
    };
    let mut actual = Vec::new();
    stdout.read_to_end(&mut actual).unwrap();
    assert_eq!(actual, expected);
}
