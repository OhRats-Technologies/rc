use crate::ohrats::rc_api_credentials::types::Administrator;
use crate::validate;

const STEP_UP_TTL_MS: u64 = 2 * 60 * 1000;

pub fn check(value: &Administrator) -> Result<(), String> {
    validate::id(&value.user_id, "administrator user id")?;
    validate::id(&value.browser_client_id, "browser client id")?;
    if value.passkey_step_up_at_ms > value.now_ms
        || value.now_ms - value.passkey_step_up_at_ms > STEP_UP_TTL_MS
    {
        return Err("fresh browser passkey step-up required".into());
    }
    Ok(())
}
