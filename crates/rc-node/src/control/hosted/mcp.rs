use super::ControlManager;
use crate::{ProcessAction, hosted_control_authority, verify_mcp_grant};
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use rc_protocol::{ControlProof, McpExecutionChunk, NodeToServer};

const STATUS_OUTPUT_LIMIT: usize = 64 * 1024;

impl ControlManager {
    #[allow(clippy::too_many_arguments)]
    pub(crate) fn mcp_status(
        &self,
        request_id: String,
        process_id: String,
        user_id: String,
        cursor: u64,
        wait_seconds: u64,
        mcp_grant: String,
        mcp_signature: String,
        control_grant: String,
        credential_id: String,
        control_assertion: String,
    ) {
        if self
            .authorize_mcp(
                &process_id,
                &user_id,
                ProcessAction::Observe,
                &mcp_grant,
                &mcp_signature,
                control_grant,
                credential_id,
                control_assertion,
            )
            .is_err()
        {
            self.emit_status_error(request_id, process_id, "process is unavailable");
            return;
        }
        let manager = self.clone();
        tokio::spawn(async move {
            let deadline =
                tokio::time::Instant::now() + std::time::Duration::from_secs(wait_seconds.min(60));
            loop {
                let Some(read) =
                    manager
                        .0
                        .processes
                        .execution_read(&process_id, cursor, STATUS_OUTPUT_LIMIT)
                else {
                    manager.emit_status_error(request_id, process_id, "process is unavailable");
                    return;
                };
                if read.status != "running"
                    || !read.chunks.is_empty()
                    || read.truncated_before > cursor
                    || tokio::time::Instant::now() >= deadline
                {
                    manager.emit(NodeToServer::McpExecutionStatusResult {
                        request_id,
                        process_id,
                        status: read.status.into(),
                        chunks: read
                            .chunks
                            .into_iter()
                            .map(|chunk| McpExecutionChunk {
                                stream: match chunk.stream {
                                    crate::StreamKind::Stdout => "stdout",
                                    crate::StreamKind::Stderr => "stderr",
                                }
                                .into(),
                                cursor: chunk.cursor,
                                data: URL_SAFE_NO_PAD.encode(chunk.bytes),
                            })
                            .collect(),
                        next_cursor: read.next_cursor,
                        truncated_before_cursor: read.truncated_before,
                        output_pending: read.more,
                        exit_code: read.exit_code,
                        signal: read.signal,
                        error: String::new(),
                    });
                    return;
                }
                tokio::time::sleep(std::time::Duration::from_millis(25)).await;
            }
        });
    }

    #[allow(clippy::too_many_arguments)]
    fn authorize_mcp(
        &self,
        process_id: &str,
        user_id: &str,
        action: ProcessAction,
        mcp_grant: &str,
        mcp_signature: &str,
        control_grant: String,
        credential_id: String,
        control_assertion: String,
    ) -> anyhow::Result<crate::ProcessPrincipal> {
        let authority = hosted_control_authority(
            &self.0.state_dir,
            &ControlProof {
                grant: control_grant,
                credential_id,
                assertion: control_assertion,
            },
            user_id,
        )?;
        let grant = verify_mcp_grant(
            &self.0.state_dir,
            mcp_grant,
            mcp_signature,
            &authority,
            user_id,
            &self.0.state.device_id,
        )?;
        if self.0.processes.execution_authority(process_id)
            != Some((crate::ProcessChannel::Mcp, grant.id))
        {
            anyhow::bail!("process is unavailable");
        }
        let principal = super::hosted_principal(user_id, &authority.role);
        let access = self.process_access(process_id, action, principal.clone())?;
        self.0
            .process_policy
            .authorize_access(access)
            .map_err(anyhow::Error::msg)?;
        Ok(principal)
    }

    fn emit_status_error(&self, request_id: String, process_id: String, error: &str) {
        self.emit(NodeToServer::McpExecutionStatusResult {
            request_id,
            process_id,
            status: "lost".into(),
            chunks: Vec::new(),
            next_cursor: 0,
            truncated_before_cursor: 0,
            output_pending: false,
            exit_code: None,
            signal: String::new(),
            error: error.into(),
        });
    }

    #[allow(clippy::too_many_arguments)]
    pub(crate) fn mcp_input(
        &self,
        request_id: String,
        process_id: String,
        user_id: String,
        data: String,
        eof: bool,
        mcp_grant: String,
        mcp_signature: String,
        control_grant: String,
        credential_id: String,
        control_assertion: String,
    ) {
        let action = if data.is_empty() {
            ProcessAction::CloseInput
        } else {
            ProcessAction::Input
        };
        let authorized = self.authorize_mcp(
            &process_id,
            &user_id,
            action,
            &mcp_grant,
            &mcp_signature,
            control_grant,
            credential_id,
            control_assertion,
        );
        let result = authorized.and_then(|_| {
            let bytes = URL_SAFE_NO_PAD.decode(data).map_err(anyhow::Error::msg)?;
            if bytes.len() > rc_protocol::PROCESS_INPUT_CHUNK_LIMIT {
                anyhow::bail!("process input exceeds transport capacity");
            }
            if !bytes.is_empty() {
                self.0.processes.input(&process_id, &bytes)?;
            }
            if eof {
                self.0.processes.close_input(&process_id);
            }
            Ok(())
        });
        self.emit_operation_result(request_id, process_id, result);
    }

    #[allow(clippy::too_many_arguments)]
    pub(crate) fn mcp_signal(
        &self,
        request_id: String,
        process_id: String,
        user_id: String,
        signal: String,
        mcp_grant: String,
        mcp_signature: String,
        control_grant: String,
        credential_id: String,
        control_assertion: String,
    ) {
        let result = self.authorize_mcp(
            &process_id,
            &user_id,
            ProcessAction::Signal,
            &mcp_grant,
            &mcp_signature,
            control_grant,
            credential_id,
            control_assertion,
        );
        let result = result.and_then(|principal| {
            let requested = crate::ProcessSignal::parse(&signal).map_err(anyhow::Error::msg)?;
            let access = self.process_access(&process_id, ProcessAction::Signal, principal)?;
            let approved = self
                .0
                .process_policy
                .authorize_signal(crate::ProcessSignalRequest {
                    access,
                    signal: requested,
                })
                .map_err(anyhow::Error::msg)?;
            self.0
                .processes
                .signal(&process_id, approved.legacy_name())
                .map_err(Into::into)
        });
        self.emit_operation_result(request_id, process_id, result);
    }

    fn emit_operation_result(
        &self,
        request_id: String,
        process_id: String,
        result: anyhow::Result<()>,
    ) {
        let error = result
            .err()
            .map(|error| error.to_string())
            .unwrap_or_default();
        self.emit(NodeToServer::McpExecutionOperationResult {
            request_id,
            process_id,
            accepted: error.is_empty(),
            error,
        });
    }
}
