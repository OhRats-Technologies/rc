use crate::AppState;
use rc_protocol::{NodeToServer, ServerToNode};
use tokio::sync::oneshot;

pub(super) async fn send(
    state: &AppState,
    device: &str,
    request_id: &str,
    receiver: oneshot::Receiver<NodeToServer>,
    message: ServerToNode,
) -> anyhow::Result<()> {
    if let Err(error) = state.nodes.send(device, &message).await {
        state.mcp.cancel_status_request(request_id);
        return Err(error);
    }
    let response = match tokio::time::timeout(std::time::Duration::from_secs(5), receiver).await {
        Ok(result) => result?,
        Err(_) => {
            state.mcp.cancel_status_request(request_id);
            anyhow::bail!("Node process operation timed out");
        }
    };
    let NodeToServer::McpExecutionOperationResult {
        accepted, error, ..
    } = response
    else {
        anyhow::bail!("invalid Node process operation response");
    };
    if !accepted {
        anyhow::bail!(if error.is_empty() {
            "Node rejected process operation".into()
        } else {
            error
        });
    }
    Ok(())
}
