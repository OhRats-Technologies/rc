package server

// mcpTools returns tool descriptors visible to a grant. Observe tokens see the
// read-only surface; terminal grants also see execution and visual tools.
func mcpTools(terminal bool) []map[string]any {
	tools := []map[string]any{
		machinesToolDescriptor(),
		processStatusToolDescriptor(),
	}
	if terminal {
		tools = []map[string]any{
			machinesToolDescriptor(),
			imageViewToolDescriptor(),
			processRunToolDescriptor(),
			processStatusToolDescriptor(),
			processInputToolDescriptor(),
			processCancelToolDescriptor(),
		}
	}
	return tools
}

func emptyObjectSchema() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false}
}

func annotations(readOnly, destructive, idempotent, openWorld bool) map[string]any {
	return map[string]any{
		"readOnlyHint":    readOnly,
		"destructiveHint": destructive,
		"idempotentHint":  idempotent,
		"openWorldHint":   openWorld,
	}
}

func machinesToolDescriptor() map[string]any {
	return map[string]any{
		"name":        "machines_list",
		"title":       "List RC machines",
		"description": "List machines granted to this agent. Call this before process_run to obtain device IDs and online state.",
		"inputSchema": emptyObjectSchema(),
		"outputSchema": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"machines": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":                 "object",
						"additionalProperties": false,
						"properties": map[string]any{
							"id":              map[string]any{"type": "string"},
							"name":            map[string]any{"type": "string"},
							"workspaceId":     map[string]any{"type": "string"},
							"hostname":        map[string]any{"type": "string"},
							"platform":        map[string]any{"type": "string"},
							"arch":            map[string]any{"type": "string"},
							"nodeVersion":     map[string]any{"type": "string"},
							"online":          map[string]any{"type": "boolean"},
							"activeProcesses": map[string]any{"type": "integer", "minimum": 0},
						},
						"required": []string{"id", "name", "workspaceId", "hostname", "platform", "arch", "nodeVersion", "online", "activeProcesses"},
					},
				},
			},
			"required": []string{"machines"},
		},
		"annotations": annotations(true, false, true, false),
	}
}

func imageViewToolDescriptor() map[string]any {
	return map[string]any{
		"name":        "image_view",
		"title":       "View an image",
		"description": "View an image file on an explicitly granted RC machine. Returns the image as MCP visual content.",
		"inputSchema": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"deviceId": map[string]any{"type": "string"},
				"path":     map[string]any{"type": "string"},
			},
			"required": []string{"deviceId", "path"},
		},
		"outputSchema": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"deviceId":  map[string]any{"type": "string"},
				"path":      map[string]any{"type": "string"},
				"mimeType":  map[string]any{"type": "string", "enum": []string{"image/png", "image/jpeg", "image/webp", "image/gif"}},
				"sizeBytes": map[string]any{"type": "integer", "minimum": 0},
			},
			"required": []string{"deviceId", "path", "mimeType", "sizeBytes"},
		},
		"annotations": annotations(true, false, true, false),
	}
}

func processRunToolDescriptor() map[string]any {
	return map[string]any{
		"name":        "process_run",
		"title":       "Run a command",
		"description": "Start one managed non-PTY execution on an explicitly granted RC machine. Provide argv for exact argument-preserving execution, or command as shell source. waitSeconds observes without changing lifetime; maxRuntimeSeconds bounds lifetime. MCP execution plaintext transits RC memory and is never persisted or logged.",
		"inputSchema": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"deviceId":          map[string]any{"type": "string"},
				"argv":              map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "minItems": 1},
				"command":           map[string]any{"type": "string"},
				"shell":             map[string]any{"type": "string", "enum": []string{"rc", "system"}, "default": "rc"},
				"cwd":               map[string]any{"type": "string"},
				"envBase":           map[string]any{"type": "string", "enum": []string{"inherit", "clean"}, "default": "inherit"},
				"env":               map[string]any{"type": "object", "additionalProperties": map[string]any{"type": []string{"string", "null"}}},
				"waitSeconds":       map[string]any{"type": "integer", "minimum": 0, "maximum": 60, "default": 20},
				"maxRuntimeSeconds": map[string]any{"type": "integer", "minimum": 1},
			},
			"required": []string{"deviceId"},
			"oneOf": []any{
				map[string]any{"required": []string{"argv"}, "not": map[string]any{"required": []string{"command"}}},
				map[string]any{"required": []string{"command"}, "not": map[string]any{"required": []string{"argv"}}},
			},
		},
		"outputSchema": mcpProcessOutputSchema(),
		"annotations":  annotations(false, true, false, true),
	}
}

func processStatusToolDescriptor() map[string]any {
	return map[string]any{
		"name":        "process_status",
		"title":       "Read process status",
		"description": "Read ordered incremental stdout/stderr and status for a process created by this same MCP grant on deviceId. Pass the previous nextCursor to avoid repeated output. waitSeconds long-polls for new output or exit.",
		"inputSchema": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"deviceId":    map[string]any{"type": "string"},
				"processId":   map[string]any{"type": "string"},
				"cursor":      map[string]any{"type": "integer", "minimum": 0},
				"waitSeconds": map[string]any{"type": "integer", "minimum": 0, "maximum": 60, "default": 0},
			},
			"required": []string{"deviceId", "processId"},
		},
		"outputSchema": mcpProcessOutputSchema(),
		"annotations":  annotations(true, false, true, false),
	}
}

func processInputToolDescriptor() map[string]any {
	return map[string]any{
		"name":        "process_input",
		"title":       "Write process input",
		"description": "Write exact text or base64-encoded bytes to stdin for a running process created by this same MCP grant and optionally close stdin. RC does not append a newline.",
		"inputSchema": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"deviceId":  map[string]any{"type": "string"},
				"processId": map[string]any{"type": "string"},
				"data":      map[string]any{"type": "string"},
				"encoding":  map[string]any{"type": "string", "enum": []string{"text", "base64"}, "default": "text"},
				"eof":       map[string]any{"type": "boolean", "default": false},
			},
			"required": []string{"deviceId", "processId"},
			"anyOf": []any{
				map[string]any{"required": []string{"data"}},
				map[string]any{"properties": map[string]any{"eof": map[string]any{"const": true}}, "required": []string{"eof"}},
			},
		},
		"outputSchema": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"deviceId":      map[string]any{"type": "string"},
				"processId":     map[string]any{"type": "string"},
				"acceptedBytes": map[string]any{"type": "integer", "minimum": 0},
				"stdinClosed":   map[string]any{"type": "boolean"},
			},
			"required": []string{"processId", "acceptedBytes", "stdinClosed"},
		},
		"annotations": annotations(false, true, false, true),
	}
}

func processCancelToolDescriptor() map[string]any {
	return map[string]any{
		"name":        "process_cancel",
		"title":       "Cancel a running process",
		"description": "Request INT, TERM, or KILL for a running process created by this same MCP grant. TERM is the normal graceful choice. Use process_status afterward to observe the final state.",
		"inputSchema": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"deviceId":  map[string]any{"type": "string"},
				"processId": map[string]any{"type": "string"},
				"signal":    map[string]any{"type": "string", "enum": []string{"INT", "TERM", "KILL"}, "default": "TERM"},
			},
			"required": []string{"deviceId", "processId"},
		},
		"outputSchema": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"processId": map[string]any{"type": "string"},
				"signal":    map[string]any{"type": "string", "enum": []string{"INT", "TERM", "KILL"}},
				"accepted":  map[string]any{"type": "boolean"},
			},
			"required": []string{"processId", "signal", "accepted"},
		},
		"annotations": annotations(false, true, false, false),
	}
}

func mcpProcessOutputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"processId": map[string]any{"type": "string"},
			"status":    map[string]any{"type": "string", "enum": []string{"running", "exited", "lost"}},
			"chunks": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"properties": map[string]any{
						"stream":   map[string]any{"type": "string", "enum": []string{"stdout", "stderr"}},
						"cursor":   map[string]any{"type": "integer", "minimum": 0},
						"encoding": map[string]any{"type": "string", "enum": []string{"text", "base64"}},
						"data":     map[string]any{"type": "string"},
					},
					"required": []string{"stream", "cursor", "encoding", "data"},
				},
			},
			"exitCode":              map[string]any{"type": []string{"integer", "null"}},
			"signal":                map[string]any{"type": []string{"string", "null"}},
			"error":                 map[string]any{"type": []string{"string", "null"}},
			"nextCursor":            map[string]any{"type": "integer", "minimum": 0},
			"outputPending":         map[string]any{"type": "boolean"},
			"truncatedBeforeCursor": map[string]any{"type": "integer", "minimum": 0},
		},
		"required": []string{"processId", "status", "chunks", "exitCode", "signal", "error", "nextCursor", "outputPending", "truncatedBeforeCursor"},
	}
}
