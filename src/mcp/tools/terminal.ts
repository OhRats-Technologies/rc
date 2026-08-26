import { runMcpProcess } from "../process";
import type { McpTool } from "./types";
import { complete } from "./types";

export const terminalRunTool: McpTool = {
  name: "process_run",
  title: "Run a command",
  description: "Run one shell command on an explicitly granted RC machine. Use machines_list for deviceId. Returns stdout/stderr plus exit or running status. MCP command and output plaintext pass through the RC server.",
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
  outputSchema: { type: "object", additionalProperties: false, properties: {
    processId: { type: "string" }, status: { type: "string", enum: ["exited", "running", "lost"] }, output: { type: "string" },
    exitCode: { type: ["integer", "null"] }, signal: { type: ["string", "null"] }, error: { type: ["string", "null"] }, nextOffset: { type: "integer" },
    outputTruncated: { type: "boolean" },
  }, required: ["processId", "status", "output", "exitCode", "signal", "error", "nextOffset", "outputTruncated"] },
  annotations: { readOnlyHint: false, destructiveHint: true, idempotentHint: false, openWorldHint: true },
  async run(context, args) {
    const result = await runMcpProcess(context, { deviceId: String(args.deviceId || ""), command: String(args.command || ""),
      cwd: String(args.cwd || ""), timeoutSeconds: Number(args.timeoutSeconds || 20) });
    const status = result.status === "exited" ? `Exit ${result.exitCode ?? "unknown"}.`
      : result.status === "running" ? `Process ${result.processId} is still running.` : `Process was lost${result.error ? `: ${result.error}` : "."}`;
    const text = result.output.trim() ? `${result.output.trimEnd()}\n${status}` : status;
    return complete(result, text);
  },
};
