import { listDevices } from "../../devices";
import { q } from "../../db";
import type { McpTool } from "./types";
import { complete } from "./types";

export const machinesListTool: McpTool = {
  name: "machines_list",
  title: "List RC machines",
  description: "List machines explicitly granted to this agent. Call this before process_run or action_run to obtain machine IDs and online state.",
  scope: "mcp:observe",
  inputSchema: { type: "object", additionalProperties: false },
  outputSchema: { type: "object", additionalProperties: false, properties: {
    machines: { type: "array", items: { type: "object", additionalProperties: false, properties: {
      id: { type: "string" }, name: { type: "string" }, workspaceId: { type: "string" }, workspace: { type: "string" },
      hostname: { type: "string" }, platform: { type: "string" }, arch: { type: "string" }, nodeVersion: { type: "string" },
      online: { type: "boolean" }, activeProcesses: { type: "integer" },
    }, required: ["id", "name", "workspaceId", "workspace", "hostname", "platform", "arch", "nodeVersion", "online", "activeProcesses"] } },
  }, required: ["machines"] },
  annotations: { readOnlyHint: true, destructiveHint: false, idempotentHint: true, openWorldHint: false },
  run(context) {
    const user = q<{ id: string; name: string }>("SELECT id,name FROM users WHERE id=?").get(context.payload.userId);
    if (!user) return complete({ machines: [] }, "No machines are available in this grant.");
    const allowed = new Set(context.payload.deviceIds);
    const machines = listDevices(user).filter(device => allowed.has(device.id)).map(device => ({
      id: device.id, name: device.name, workspaceId: device.workspace_id, workspace: device.workspace_name,
      hostname: device.hostname, platform: device.platform, arch: device.arch, nodeVersion: device.agent_version,
      online: device.online, activeProcesses: device.active_processes,
    }));
    const text = machines.length ? machines.map(machine => `${machine.name} — ${machine.online ? "online" : "offline"} — ${machine.platform}/${machine.arch} — workspace ${machine.workspace} — node ${machine.nodeVersion} — id ${machine.id}`).join("\n")
      : "No machines are available in this grant.";
    return complete({ machines }, text);
  },
};
