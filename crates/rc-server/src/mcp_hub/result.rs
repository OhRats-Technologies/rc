#[derive(Debug, Clone, serde::Serialize)]
#[serde(rename_all = "camelCase")]
pub struct McpOutputChunk {
    pub stream: &'static str,
    pub cursor: usize,
    pub encoding: &'static str,
    pub data: String,
}

#[derive(Debug, Clone, serde::Serialize)]
#[serde(rename_all = "camelCase")]
pub struct McpProcessResult {
    pub process_id: String,
    pub status: String,
    pub chunks: Vec<McpOutputChunk>,
    pub exit_code: Option<i32>,
    pub signal: Option<String>,
    pub error: Option<String>,
    pub next_cursor: usize,
    pub output_pending: bool,
    pub truncated_before_cursor: usize,
}
