use crate::{AppState, UserIdentity, now_ms};
use base64::{
    Engine as _,
    engine::general_purpose::{STANDARD, URL_SAFE_NO_PAD},
};
use rusqlite::OptionalExtension;
use serde::{Deserialize, Serialize};
use uuid::Uuid;
use webauthn_rs::prelude::{
    CreationChallengeResponse, Passkey, PasskeyAuthentication, PasskeyRegistration,
    PublicKeyCredential, RegisterPublicKeyCredential, RequestChallengeResponse,
};

const CEREMONY_TTL_MS: i64 = 5 * 60 * 1000;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum RegistrationKind {
    Setup,
    Register,
    AddPasskey,
}

impl RegistrationKind {
    fn as_str(self) -> &'static str {
        match self {
            Self::Setup => "setup",
            Self::Register => "register",
            Self::AddPasskey => "add-passkey",
        }
    }
}

#[derive(Debug, Serialize, Deserialize)]
struct RegistrationMeta {
    name: String,
    invite_id: Option<String>,
}

pub struct FinishedRegistration {
    pub user_id: String,
    pub name: String,
    pub invite_id: Option<String>,
    pub passkey: Passkey,
}

pub fn start_registration(
    state: &AppState,
    kind: RegistrationKind,
    user_id: &str,
    name: &str,
    invite_id: Option<&str>,
) -> anyhow::Result<(String, CreationChallengeResponse)> {
    let excludes = if kind == RegistrationKind::AddPasskey {
        Some(
            load_passkeys(state, Some(user_id))?
                .into_iter()
                .map(|(_, _, passkey)| passkey.cred_id().clone())
                .collect(),
        )
    } else {
        None
    };
    let user_uuid = Uuid::parse_str(user_id).unwrap_or_else(|_| Uuid::new_v4());
    let (options, registration) = state
        .webauthn
        .start_passkey_registration(user_uuid, name, name, excludes)?;
    let ceremony_id = Uuid::new_v4().to_string();
    let meta = RegistrationMeta {
        name: name.to_owned(),
        invite_id: invite_id.map(str::to_owned),
    };
    let meta_json = serde_json::to_string(&meta)?;
    let registration_json = serde_json::to_string(&registration)?;
    state.db.with_connection(|db| {
        db.execute(
            "INSERT INTO ceremonies(id,kind,user_id,meta_json,state_json,expires_at) VALUES(?,?,?,?,?,?)",
            rusqlite::params![
                ceremony_id,
                kind.as_str(),
                user_id,
                meta_json,
                registration_json,
                now_ms() + CEREMONY_TTL_MS
            ],
        )?;
        Ok(())
    })?;
    Ok((ceremony_id, options))
}

pub fn finish_registration(
    state: &AppState,
    kind: RegistrationKind,
    ceremony_id: &str,
    response: serde_json::Value,
) -> anyhow::Result<FinishedRegistration> {
    let ceremony = take_ceremony(state, ceremony_id, kind.as_str())?
        .ok_or_else(|| anyhow::anyhow!("registration expired"))?;
    let user_id = ceremony
        .user_id
        .ok_or_else(|| anyhow::anyhow!("registration expired"))?;
    let meta: RegistrationMeta = serde_json::from_str(&ceremony.meta_json)?;
    let registration: PasskeyRegistration = serde_json::from_str(&ceremony.state_json)?;
    let response: RegisterPublicKeyCredential = serde_json::from_value(response)?;
    let passkey = state
        .webauthn
        .finish_passkey_registration(&response, &registration)?;
    Ok(FinishedRegistration {
        user_id,
        name: meta.name,
        invite_id: meta.invite_id,
        passkey,
    })
}

pub fn start_login(state: &AppState) -> anyhow::Result<(String, RequestChallengeResponse)> {
    let passkeys = load_passkeys(state, None)?;
    if passkeys.is_empty() {
        anyhow::bail!("no passkeys registered");
    }
    let credentials: Vec<_> = passkeys
        .into_iter()
        .map(|(_, _, passkey)| passkey)
        .collect();
    let (options, authentication) = state.webauthn.start_passkey_authentication(&credentials)?;
    let ceremony_id = Uuid::new_v4().to_string();
    let authentication_json = serde_json::to_string(&authentication)?;
    state.db.with_connection(|db| {
        db.execute(
            "INSERT INTO ceremonies(id,kind,meta_json,state_json,expires_at) VALUES(?,'login','{}',?,?)",
            rusqlite::params![
                ceremony_id,
                authentication_json,
                now_ms() + CEREMONY_TTL_MS
            ],
        )?;
        Ok(())
    })?;
    Ok((ceremony_id, options))
}

pub fn finish_login(
    state: &AppState,
    ceremony_id: &str,
    response: serde_json::Value,
) -> anyhow::Result<UserIdentity> {
    let ceremony = take_ceremony(state, ceremony_id, "login")?
        .ok_or_else(|| anyhow::anyhow!("authentication expired"))?;
    let authentication: PasskeyAuthentication = serde_json::from_str(&ceremony.state_json)?;
    let response: PublicKeyCredential = serde_json::from_value(response)?;
    let result = state
        .webauthn
        .finish_passkey_authentication(&response, &authentication)?;
    let credential_id = URL_SAFE_NO_PAD.encode(result.cred_id().as_ref());
    let row =
        find_passkey(state, &credential_id)?.ok_or_else(|| anyhow::anyhow!("unknown passkey"))?;
    let mut passkey: Passkey = serde_json::from_str(&row.credential_json)?;
    let _ = passkey.update_credential(&result);
    let passkey_json = serde_json::to_string(&passkey)?;
    state.db.with_connection(|db| {
        db.execute(
            "UPDATE passkeys SET credential_json=?,last_used=? WHERE id=?",
            rusqlite::params![passkey_json, now_ms(), row.id],
        )?;
        Ok(())
    })?;
    crate::user_by_id(state, &row.user_id)?.ok_or_else(|| anyhow::anyhow!("account unavailable"))
}

pub fn insert_passkey(
    transaction: &rusqlite::Transaction<'_>,
    user_id: &str,
    passkey: &Passkey,
) -> rusqlite::Result<String> {
    let id = Uuid::new_v4().to_string();
    let credential_json = serde_json::to_string(passkey)
        .map_err(|error| rusqlite::Error::ToSqlConversionFailure(Box::new(error)))?;
    transaction.execute(
        "INSERT INTO passkeys(id,user_id,name,credential_json,created_at) VALUES(?,?,?,?,?)",
        rusqlite::params![id, user_id, "Passkey", credential_json, now_ms()],
    )?;
    Ok(id)
}

pub fn passkey_authority_material(passkey: &Passkey) -> anyhow::Result<(String, String)> {
    Ok((
        URL_SAFE_NO_PAD.encode(passkey.cred_id().as_ref()),
        STANDARD.encode(serde_json::to_vec(passkey.get_public_key())?),
    ))
}

pub fn user_passkeys(state: &AppState, user_id: &str) -> anyhow::Result<Vec<Passkey>> {
    Ok(load_passkeys(state, Some(user_id))?
        .into_iter()
        .map(|(_, _, passkey)| passkey)
        .collect())
}

pub fn passkey_public_key(
    state: &AppState,
    user_id: &str,
    credential_id: &str,
) -> anyhow::Result<Option<String>> {
    for (_, candidate_user, passkey) in load_passkeys(state, Some(user_id))? {
        if candidate_user == user_id {
            let (id, public_key) = passkey_authority_material(&passkey)?;
            if id == credential_id {
                return Ok(Some(public_key));
            }
        }
    }
    Ok(None)
}

struct CeremonyRow {
    user_id: Option<String>,
    meta_json: String,
    state_json: String,
}

fn take_ceremony(state: &AppState, id: &str, kind: &str) -> rusqlite::Result<Option<CeremonyRow>> {
    state.db.with_connection_mut(|db| {
        let tx = db.transaction()?;
        let row = tx
            .query_row(
                "SELECT user_id,meta_json,state_json FROM ceremonies WHERE id=? AND kind=? AND expires_at>?",
                rusqlite::params![id, kind, now_ms()],
                |row| {
                    Ok(CeremonyRow {
                        user_id: row.get(0)?,
                        meta_json: row.get(1)?,
                        state_json: row.get(2)?,
                    })
                },
            )
            .optional()?;
        tx.execute("DELETE FROM ceremonies WHERE id=?", [id])?;
        tx.commit()?;
        Ok(row)
    })
}

struct PasskeyRow {
    id: String,
    user_id: String,
    credential_json: String,
}

fn find_passkey(state: &AppState, credential_id: &str) -> anyhow::Result<Option<PasskeyRow>> {
    for (id, user_id, passkey) in load_passkeys(state, None)? {
        if URL_SAFE_NO_PAD.encode(passkey.cred_id().as_ref()) == credential_id {
            return Ok(Some(PasskeyRow {
                id,
                user_id,
                credential_json: serde_json::to_string(&passkey)?,
            }));
        }
    }
    Ok(None)
}

fn load_passkeys(
    state: &AppState,
    user_id: Option<&str>,
) -> anyhow::Result<Vec<(String, String, Passkey)>> {
    let rows = state.db.with_connection(|db| {
        let sql = if user_id.is_some() {
            "SELECT id,user_id,credential_json FROM passkeys WHERE user_id=? ORDER BY created_at"
        } else {
            "SELECT id,user_id,credential_json FROM passkeys ORDER BY created_at"
        };
        let mut statement = db.prepare(sql)?;
        let values = if let Some(user_id) = user_id {
            statement
                .query_map([user_id], |row| {
                    Ok((
                        row.get::<_, String>(0)?,
                        row.get::<_, String>(1)?,
                        row.get::<_, String>(2)?,
                    ))
                })?
                .collect::<Result<Vec<_>, _>>()?
        } else {
            statement
                .query_map([], |row| {
                    Ok((
                        row.get::<_, String>(0)?,
                        row.get::<_, String>(1)?,
                        row.get::<_, String>(2)?,
                    ))
                })?
                .collect::<Result<Vec<_>, _>>()?
        };
        Ok(values)
    })?;
    rows.into_iter()
        .map(|(id, user_id, json)| Ok((id, user_id, serde_json::from_str(&json)?)))
        .collect()
}
