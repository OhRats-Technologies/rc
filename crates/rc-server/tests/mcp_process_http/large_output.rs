use super::support::{Harness, recv, respond_status, rpc_call};
use rc_node::ServerTransport;
use rc_protocol::ServerToNode;

#[tokio::test]
async fn large_control_frames_do_not_poison_the_next_execution() -> anyhow::Result<()> {
    let harness = Harness::start().await?;
    let client = reqwest::Client::new();
    let mut node = ServerTransport::connect(&harness.base, &harness.node).await?;
    for id in 1..=2 {
        let command = "x".repeat(70_000);
        let output = vec![b'x'; 64 * 1024];
        let call = rpc_call(
            &client,
            &harness,
            "process_run",
            serde_json::json!({
                "deviceId": harness.device_id, "command": command, "waitSeconds": 0
            }),
            id,
        );
        let exchange = async {
            let ServerToNode::McpStart { process_id, .. } = recv(&mut node).await? else {
                anyhow::bail!("missing start");
            };
            respond_status(
                &mut node,
                &process_id,
                "exited",
                vec![("stdout", output.clone())],
                Some(0),
                "",
            )
            .await
        };
        let (result, ()) = tokio::try_join!(call, exchange)?;
        assert_eq!(
            result["result"]["structuredContent"]["nextCursor"],
            output.len()
        );
        assert_eq!(
            result["result"]["structuredContent"]["chunks"][0]["data"],
            String::from_utf8(output)?
        );
    }
    node.close().await;
    Ok(())
}
