pub(super) fn machines_descriptor() -> serde_json::Value {
    serde_json::json!({
        "name": "machines_list",
        "title": "List RC machines",
        "description": "List machines explicitly granted to this agent. Call this before process_run to obtain machine IDs and online state.",
        "inputSchema": {
            "type": "object",
            "additionalProperties": false,
        },
        "outputSchema": {
            "type": "object",
            "additionalProperties": false,
            "properties": {
                "machines": {
                    "type": "array",
                    "items": {
                        "type": "object",
                        "additionalProperties": false,
                        "properties": {
                            "id": {"type": "string"},
                            "name": {"type": "string"},
                            "workspaceId": {"type": "string"},
                            "workspace": {"type": "string"},
                            "hostname": {"type": "string"},
                            "platform": {"type": "string"},
                            "arch": {"type": "string"},
                            "nodeVersion": {"type": "string"},
                            "online": {"type": "boolean"},
                            "activeProcesses": {"type": "integer", "minimum": 0},
                        },
                        "required": ["id", "name", "workspaceId", "workspace", "hostname", "platform", "arch", "nodeVersion", "online", "activeProcesses"],
                    },
                },
            },
            "required": ["machines"],
        },
        "annotations": {
            "readOnlyHint": true,
            "destructiveHint": false,
            "idempotentHint": true,
            "openWorldHint": false,
        },
    })
}

pub(super) fn run_descriptor() -> serde_json::Value {
    serde_json::json!({
        "name": "process_run",
        "title": "Run a command",
        "description": "Run one shell command on an explicitly granted RC machine. Use machines_list for deviceId. Returns stdout/stderr plus exit or running status. MCP command and output plaintext pass through the RC server.",
        "inputSchema": {
            "type": "object",
            "additionalProperties": false,
            "properties": {
                "deviceId": {"type": "string"},
                "command": {"type": "string", "minLength": 1, "maxLength": 8192},
                "cwd": {"type": "string", "maxLength": 4096},
                "timeoutSeconds": {
                    "type": "integer",
                    "minimum": 1,
                    "maximum": 60,
                    "default": 20,
                },
            },
            "required": ["deviceId", "command"],
        },
        "outputSchema": process_output_schema(),
        "annotations": {
            "readOnlyHint": false,
            "destructiveHint": true,
            "idempotentHint": false,
            "openWorldHint": true,
        },
    })
}

pub(super) fn status_descriptor() -> serde_json::Value {
    serde_json::json!({
        "name": "process_status",
        "title": "Read process status",
        "description": "Read incremental output and status for a process created by this same MCP grant. Pass the prior nextOffset to avoid repeated output. waitSeconds can wait for new output or exit. Buffers are bounded and ephemeral.",
        "inputSchema": {
            "type": "object",
            "additionalProperties": false,
            "properties": {
                "processId": {"type": "string"},
                "offset": {"type": "integer", "minimum": 0},
                "waitSeconds": {
                    "type": "integer",
                    "minimum": 0,
                    "maximum": 60,
                    "default": 0,
                },
            },
            "required": ["processId"],
        },
        "outputSchema": process_output_schema(),
        "annotations": {
            "readOnlyHint": true,
            "destructiveHint": false,
            "idempotentHint": true,
            "openWorldHint": false,
        },
    })
}

fn process_output_schema() -> serde_json::Value {
    serde_json::json!({
        "type": "object",
        "additionalProperties": false,
        "properties": {
            "processId": {"type": "string"},
            "status": {"type": "string", "enum": ["running", "exited", "lost"]},
            "output": {"type": "string"},
            "exitCode": {"type": ["integer", "null"]},
            "signal": {"type": ["string", "null"]},
            "error": {"type": ["string", "null"]},
            "nextOffset": {"type": "integer", "minimum": 0},
            "outputTruncated": {"type": "boolean"},
        },
        "required": ["processId", "status", "output", "exitCode", "signal", "error", "nextOffset", "outputTruncated"],
    })
}

#[cfg(test)]
mod tests {
    use super::{machines_descriptor, run_descriptor, status_descriptor};

    #[test]
    fn structured_tools_publish_output_schemas() {
        for descriptor in [machines_descriptor(), run_descriptor(), status_descriptor()] {
            assert!(descriptor.get("outputSchema").is_some());
        }
    }
}
