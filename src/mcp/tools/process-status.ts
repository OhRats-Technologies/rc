import { mcpProcessStatus, type McpProcessResult } from "../process";
import type { McpTool } from "./types";
import { complete } from "./types";

export const processStatusTool: McpTool = {
  name: "process_status",
  title: "Read process status",
  description: "Read incremental output and status for a process created by this same MCP grant. Pass the prior nextOffset to avoid repeated output. waitSeconds can wait for new output or exit. Buffers are bounded and ephemeral.",
  scope: "mcp:observe",
  inputSchema: { type: "object", additionalProperties: false, properties: {
    processId: { type: "string", description: "Process ID returned by process_run or action_run." },
    offset: { type: "integer", minimum: 0, description: "Previous nextOffset. Default 0." },
    waitSeconds: { type: "integer", minimum: 0, maximum: 60, default: 0, description: "Wait for new output or exit. Default 0." },
  }, required: ["processId"] },
  outputSchema: { type: "object", additionalProperties: false, properties: {
    processId: { type: "string" }, status: { type: "string", enum: ["exited", "running", "lost"] }, output: { type: "string" },
    exitCode: { type: ["integer", "null"] }, signal: { type: ["string", "null"] }, error: { type: ["string", "null"] }, nextOffset: { type: "integer" },
    outputTruncated: { type: "boolean" },
  }, required: ["processId", "status", "output", "exitCode", "signal", "error", "nextOffset", "outputTruncated"] },
  annotations: { readOnlyHint: true, destructiveHint: false, idempotentHint: true, openWorldHint: false },
  async run(context, args) {
    const result = await mcpProcessStatus(context, String(args.processId || ""), Number(args.offset || 0), Number(args.waitSeconds || 0));
    return complete(result, processText(result));
  },
};

function processText(result: McpProcessResult) {
  const status = result.status === "exited" ? `Exit ${result.exitCode ?? "unknown"}.`
    : result.status === "running" ? `Process ${result.processId} is still running.` : `Process was lost${result.error ? `: ${result.error}` : "."}`;
  const suffix = result.outputTruncated ? `${status} Output buffer is truncated.` : status;
  return result.output.trim() ? `${result.output.trimEnd()}\n${suffix}` : suffix;
}
