pub(super) fn machines_descriptor() -> serde_json::Value {
    serde_json::json!({
        "name": "machines_list",
        "title": "List RC machines",
        "description": "List machines explicitly granted to this agent. Call this before process_run to obtain device IDs and online state.",
        "inputSchema": empty_object_schema(),
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
        "annotations": annotations(true, false, true, false),
    })
}

pub(super) fn image_descriptor() -> serde_json::Value {
    serde_json::json!({
        "name": "image_view",
        "title": "View an image",
        "description": "View a local image file on an explicitly granted RC machine. Returns the image as MCP visual content. Supports PNG, JPEG, WebP, and GIF up to 8 MiB.",
        "inputSchema": {
            "type": "object",
            "additionalProperties": false,
            "properties": {
                "deviceId": {"type": "string"},
                "path": {"type": "string"},
            },
            "required": ["deviceId", "path"],
        },
        "outputSchema": {
            "type": "object",
            "additionalProperties": false,
            "properties": {
                "deviceId": {"type": "string"},
                "path": {"type": "string"},
                "mimeType": {"type": "string", "enum": ["image/png", "image/jpeg", "image/webp", "image/gif"]},
                "sizeBytes": {"type": "integer", "minimum": 0},
            },
            "required": ["deviceId", "path", "mimeType", "sizeBytes"],
        },
        "annotations": annotations(true, false, true, false),
    })
}

pub(super) fn run_descriptor() -> serde_json::Value {
    serde_json::json!({
        "name": "process_run",
        "title": "Run a command",
        "description": "Start one non-PTY shell command on an explicitly granted RC machine. waitSeconds controls how long this call waits for output or exit; it is not a process runtime limit. Use process_status for continued incremental output. MCP command and output plaintext pass through RC memory and are never persisted.",
        "inputSchema": {
            "type": "object",
            "additionalProperties": false,
            "properties": {
                "deviceId": {"type": "string"},
                "command": {"type": "string"},
                "cwd": {"type": "string"},
                "waitSeconds": {
                    "type": "integer",
                    "minimum": 0,
                    "maximum": 60,
                    "default": 20,
                },
            },
            "required": ["deviceId", "command"],
        },
        "outputSchema": process_output_schema(),
        "annotations": annotations(false, true, false, true),
    })
}

pub(super) fn status_descriptor() -> serde_json::Value {
    serde_json::json!({
        "name": "process_status",
        "title": "Read process status",
        "description": "Read ordered incremental stdout/stderr and status for a process created by this same MCP grant. Pass the previous nextCursor to avoid repeated output. waitSeconds long-polls for new output or exit. The rolling buffer is bounded and ephemeral.",
        "inputSchema": {
            "type": "object",
            "additionalProperties": false,
            "properties": {
                "processId": {"type": "string"},
                "cursor": {"type": "integer", "minimum": 0},
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
        "annotations": annotations(true, false, true, false),
    })
}

pub(super) fn input_descriptor() -> serde_json::Value {
    serde_json::json!({
        "name": "process_input",
        "title": "Write process input",
        "description": "Write exact UTF-8 text to stdin for a running process created by this same MCP grant and optionally close stdin. RC does not append a newline; include \\n in data for line-oriented programs. Requests are limited by the actual RC process-input transport chunk, not an arbitrary string-length schema.",
        "inputSchema": {
            "type": "object",
            "additionalProperties": false,
            "properties": {
                "processId": {"type": "string"},
                "data": {"type": "string"},
                "eof": {"type": "boolean", "default": false},
            },
            "required": ["processId"],
            "anyOf": [
                {"required": ["data"]},
                {"properties": {"eof": {"const": true}}, "required": ["eof"]},
            ],
        },
        "outputSchema": {
            "type": "object",
            "additionalProperties": false,
            "properties": {
                "processId": {"type": "string"},
                "acceptedBytes": {"type": "integer", "minimum": 0},
                "stdinClosed": {"type": "boolean"},
            },
            "required": ["processId", "acceptedBytes", "stdinClosed"],
        },
        "annotations": annotations(false, true, false, true),
    })
}

pub(super) fn cancel_descriptor() -> serde_json::Value {
    serde_json::json!({
        "name": "process_cancel",
        "title": "Cancel a running process",
        "description": "Request INT, TERM, or KILL for a running process created by this same MCP grant. TERM is the normal graceful choice. Use process_status afterward to observe the final state.",
        "inputSchema": {
            "type": "object",
            "additionalProperties": false,
            "properties": {
                "processId": {"type": "string"},
                "signal": {
                    "type": "string",
                    "enum": ["INT", "TERM", "KILL"],
                    "default": "TERM",
                },
            },
            "required": ["processId"],
        },
        "outputSchema": {
            "type": "object",
            "additionalProperties": false,
            "properties": {
                "processId": {"type": "string"},
                "signal": {"type": "string", "enum": ["INT", "TERM", "KILL"]},
                "accepted": {"type": "boolean"},
            },
            "required": ["processId", "signal", "accepted"],
        },
        "annotations": annotations(false, true, false, false),
    })
}

fn process_output_schema() -> serde_json::Value {
    serde_json::json!({
        "type": "object",
        "additionalProperties": false,
        "properties": {
            "processId": {"type": "string"},
            "status": {"type": "string", "enum": ["running", "exited", "lost"]},
            "chunks": {
                "type": "array",
                "items": {
                    "type": "object",
                    "additionalProperties": false,
                    "properties": {
                        "stream": {"type": "string", "enum": ["stdout", "stderr"]},
                        "text": {"type": "string"},
                    },
                    "required": ["stream", "text"],
                },
            },
            "exitCode": {"type": ["integer", "null"]},
            "signal": {"type": ["string", "null"]},
            "error": {"type": ["string", "null"]},
            "nextCursor": {"type": "integer", "minimum": 0},
            "outputPending": {"type": "boolean"},
            "truncatedBeforeCursor": {"type": "integer", "minimum": 0},
        },
        "required": ["processId", "status", "chunks", "exitCode", "signal", "error", "nextCursor", "outputPending", "truncatedBeforeCursor"],
    })
}

fn empty_object_schema() -> serde_json::Value {
    serde_json::json!({"type": "object", "additionalProperties": false})
}

fn annotations(
    read_only: bool,
    destructive: bool,
    idempotent: bool,
    open_world: bool,
) -> serde_json::Value {
    serde_json::json!({
        "readOnlyHint": read_only,
        "destructiveHint": destructive,
        "idempotentHint": idempotent,
        "openWorldHint": open_world,
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn structured_tools_publish_exact_unbounded_string_schemas() {
        let descriptors = [
            machines_descriptor(),
            image_descriptor(),
            run_descriptor(),
            status_descriptor(),
            input_descriptor(),
            cancel_descriptor(),
        ];
        for descriptor in descriptors {
            assert!(descriptor.get("outputSchema").is_some());
            assert!(!contains_key(&descriptor, "minLength"));
            assert!(!contains_key(&descriptor, "maxLength"));
        }
    }

    fn contains_key(value: &serde_json::Value, key: &str) -> bool {
        match value {
            serde_json::Value::Object(object) => {
                object.contains_key(key) || object.values().any(|value| contains_key(value, key))
            }
            serde_json::Value::Array(values) => values.iter().any(|value| contains_key(value, key)),
            _ => false,
        }
    }
}
