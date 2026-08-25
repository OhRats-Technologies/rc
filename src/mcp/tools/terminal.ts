import { runMcpProcess } from "../process";
import type { McpTool } from "./types";
import { complete } from "./types";

export const terminalRunTool: McpTool = {
  name: "process_run",
  title: "Run a command",
  description: "Run one shell command on an explicitly granted RC machine. MCP command and output plaintext pass through the RC server.",
  scope: "mcp:terminal",
  inputSchema: {
    type: "object", additionalProperties: false,
    properties: {
      deviceId: { type: "string", description: "Machine ID from machines_list" },
      command: { type: "string", minLength: 1, maxLength: 8192 },
      cwd: { type: "string", maxLength: 4096 },
      timeoutSeconds: { type: "integer", minimum: 1, maximum: 60, default: 20 },
    }, required: ["deviceId", "command"],
  },
  async run(context, args) {
    const result = await runMcpProcess(context, { deviceId: String(args.deviceId || ""), command: String(args.command || ""),
      cwd: String(args.cwd || ""), kind: "terminal", timeoutSeconds: Number(args.timeoutSeconds || 20) });
    return complete(result);
  },
};
