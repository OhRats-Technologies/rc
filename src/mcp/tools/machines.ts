import { listDevices } from "../../devices";
import { q } from "../../db";
import type { McpTool } from "./types";
import { complete } from "./types";

export const machinesListTool: McpTool = {
  name: "machines_list",
  title: "List RC machines",
  description: "List the machines explicitly granted to this agent, including workspace, platform, version, and online state.",
  scope: "mcp:observe",
  inputSchema: { type: "object", additionalProperties: false },
  run(context) {
    const user = q<{ id: string; name: string }>("SELECT id,name FROM users WHERE id=?").get(context.payload.userId);
    if (!user) return complete([]);
    const allowed = new Set(context.payload.deviceIds);
    const machines = listDevices(user).filter(device => allowed.has(device.id)).map(device => ({
      id: device.id, name: device.name, workspaceId: device.workspace_id, workspace: device.workspace_name,
      hostname: device.hostname, platform: device.platform, arch: device.arch, nodeVersion: device.agent_version,
      online: device.online, activeProcesses: device.active_processes,
    }));
    return complete(machines);
  },
};
