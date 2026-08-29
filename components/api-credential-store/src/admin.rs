use crate::ohrats::rc_identity::{
    admin_consumer::{self, Claim},
    types::HumanAuthorization,
};

pub fn check(value: HumanAuthorization, operation: &str) -> Result<Claim, String> {
    admin_consumer::consume(&value, operation)
}
