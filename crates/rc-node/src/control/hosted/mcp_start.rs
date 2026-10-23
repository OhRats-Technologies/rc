use super::{ControlManager, hosted_principal, lifetime, mcp_failures};
use crate::{
    ProcessChannel, ProcessLifetime, ProcessSpec, ProcessStartRequest, hosted_control_authority,
    verify_mcp_grant,
};
use rc_protocol::{ControlProof, NodeToServer};

impl ControlManager {
    #[allow(clippy::too_many_arguments)]
    pub(in crate::control) fn mcp_start(
        &self,
        process_id: String,
        user_id: String,
        mode: rc_protocol::ExecutionMode,
        cwd: String,
        environment: rc_protocol::EnvironmentSpec,
        max_runtime_seconds: Option<u64>,
        mcp_grant: String,
        mcp_signature: String,
        control_grant: String,
        credential_id: String,
        control_assertion: String,
    ) {
        let proof = ControlProof {
            grant: control_grant,
            credential_id,
            assertion: control_assertion,
        };
        let Ok(authority) = hosted_control_authority(&self.0.state_dir, &proof, &user_id) else {
            self.emit(NodeToServer::McpExit {
                process_id,
                exit_code: 126,
                signal: String::new(),
            });
            return;
        };
        let Ok(mcp_grant) = verify_mcp_grant(
            &self.0.state_dir,
            &mcp_grant,
            &mcp_signature,
            &authority,
            &user_id,
            &self.0.state.device_id,
        ) else {
            self.emit(NodeToServer::McpExit {
                process_id,
                exit_code: 126,
                signal: String::new(),
            });
            return;
        };
        let principal = hosted_principal(&user_id, &authority.role);
        let max_runtime_ms = lifetime::bounded_runtime_ms(
            max_runtime_seconds,
            mcp_grant.expires_at,
            crate::lock::now_ms(),
        );
        let plan = match self.0.process_policy.authorize_start(ProcessStartRequest {
            execution_id: process_id.clone(),
            mode: crate::control::direct::execution_mode(mode),
            cwd: (!cwd.is_empty()).then_some(cwd),
            environment: crate::control::direct::process_environment(environment),
            terminal: None,
            channel: ProcessChannel::Mcp,
            lifetime: ProcessLifetime::Managed,
            principal: principal.clone(),
            max_runtime_ms,
        }) {
            Ok(value) => value,
            Err(_) => {
                self.0.mcp_start_failures.lock().record(
                    &process_id,
                    &user_id,
                    &mcp_grant.id,
                    mcp_failures::StartFailure::Policy,
                    std::time::Instant::now(),
                );
                self.emit(NodeToServer::McpExit {
                    process_id,
                    exit_code: 126,
                    signal: String::new(),
                });
                return;
            }
        };
        let spec = ProcessSpec {
            id: process_id.clone(),
            mode: plan.mode,
            environment: plan.environment,
            cwd: plan.cwd.unwrap_or_default(),
            terminal: plan.terminal,
            session_id: String::new(),
            user_id,
            authorization_id: mcp_grant.id.clone(),
            secure: false,
            relay_id: format!("mcp:{process_id}"),
            scrollback_bytes: plan.scrollback_bytes,
            stdin_chunk_bytes: plan.stdin_chunk_bytes,
            terminate_grace_ms: plan.terminate_grace_ms,
            reattach_grace_ms: plan.reattach_grace_ms,
            lifetime: ProcessLifetime::Managed,
            channel: ProcessChannel::Mcp,
            principal: principal.clone(),
            max_runtime_ms: plan.max_runtime_ms,
        };
        let result = self.0.processes.start(spec);
        if let Err(error) = &result {
            self.0.mcp_start_failures.lock().record(
                &process_id,
                &principal.user_id,
                &mcp_grant.id,
                mcp_failures::StartFailure::runtime(error),
                std::time::Instant::now(),
            );
        }
        if !matches!(result, Ok(true)) {
            self.emit(NodeToServer::McpExit {
                process_id,
                exit_code: 127,
                signal: String::new(),
            });
        }
    }
}
