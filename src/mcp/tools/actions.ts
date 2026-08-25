import { q, id, now } from "../../db";
import { actionHash } from "../grants";
import { runMcpProcess } from "../process";
import type { McpTool } from "./types";
import { complete } from "./types";

function allowedActions(context: Parameters<McpTool["run"]>[0]) {
  return new Map(context.payload.actions.map(action => [action.id, action.hash]));
}

export const actionsListTool: McpTool = {
  name: "actions_list",
  title: "List saved Actions",
  description: "List saved RC Actions captured when this agent grant was approved. Edited or newly created Actions require re-authorization.",
  scope: "mcp:actions",
  inputSchema: { type: "object", additionalProperties: false },
  outputSchema: { type: "object", additionalProperties: false, properties: {
    actions: { type: "array", items: { type: "object", additionalProperties: false, properties: {
      id: { type: "string" }, workspaceId: { type: "string" }, workspace: { type: "string" }, name: { type: "string" },
      description: { type: "string" }, confirmationRequired: { type: "boolean" },
    }, required: ["id", "workspaceId", "workspace", "name", "description", "confirmationRequired"] } },
  }, required: ["actions"] },
  annotations: { readOnlyHint: true, destructiveHint: false, idempotentHint: true, openWorldHint: false },
  run(context) {
    const allowed = allowedActions(context), deviceIds = context.payload.deviceIds;
    if (!deviceIds.length || !allowed.size) return complete({ actions: [] }, "No saved Actions are available in this grant.");
    const workspaceRows = q<{ workspace_id: string }>(`SELECT DISTINCT workspace_id FROM devices WHERE id IN (${deviceIds.map(() => "?").join(",")})`).all(...deviceIds);
    if (!workspaceRows.length) return complete({ actions: [] }, "No saved Actions are available in this grant.");
    const rows = q<any>(`SELECT a.id,a.workspace_id,w.name workspace_name,a.name,a.description,a.command,a.cwd,a.confirm
      FROM actions a JOIN workspaces w ON w.id=a.workspace_id WHERE a.workspace_id IN (${workspaceRows.map(() => "?").join(",")}) ORDER BY w.name,a.name`)
      .all(...workspaceRows.map(row => row.workspace_id));
    const actions = rows.filter(action => allowed.get(action.id) === actionHash(action.command, action.cwd)).map(action => ({
      id: action.id, workspaceId: action.workspace_id, workspace: action.workspace_name, name: action.name,
      description: action.description, confirmationRequired: Boolean(action.confirm),
    }));
    const text = actions.length ? actions.map(action => `${action.name} — workspace ${action.workspace} — id ${action.id}${action.confirmationRequired ? " — confirmation required" : ""}`).join("\n")
      : "No saved Actions are available in this grant.";
    return complete({ actions }, text);
  },
};

function confirmation(context: Parameters<McpTool["run"]>[0], actionId: string, deviceIds: string[], params: Record<string, unknown>) {
  const state = String(params.requestState || ""), response = (params.inputResponses as any)?.confirm;
  if (state && response?.action === "accept" && response?.content?.confirm === true) {
    const row = q<any>("SELECT * FROM mcp_confirmations WHERE id=? AND grant_id=? AND action_id=? AND expires_at>?")
      .get(state, context.payload.id, actionId, now());
    if (row && JSON.stringify(JSON.parse(row.device_ids)) === JSON.stringify(deviceIds)) {
      q("DELETE FROM mcp_confirmations WHERE id=?").run(state); return null;
    }
  }
  const requestState = id();
  q("INSERT INTO mcp_confirmations(id,grant_id,action_id,device_ids,expires_at) VALUES(?,?,?,?,?)")
    .run(requestState, context.payload.id, actionId, JSON.stringify(deviceIds), now() + 5 * 60_000);
  return { resultType: "input_required", requestState, inputRequests: { confirm: {
    method: "elicitation/create", params: { mode: "form", message: "This RC Action requires confirmation before it runs.",
      requestedSchema: { type: "object", properties: { confirm: { type: "boolean", title: "Run this Action" } }, required: ["confirm"] } },
  } } };
}

export const actionRunTool: McpTool = {
  name: "action_run",
  title: "Run a saved Action",
  description: "Run one saved Action on one or more machines in its workspace. Actions marked for confirmation request human approval in-band.",
  scope: "mcp:actions",
  inputSchema: { type: "object", additionalProperties: false, properties: {
    actionId: { type: "string" }, deviceIds: { type: "array", minItems: 1, maxItems: 25, items: { type: "string" } },
    timeoutSeconds: { type: "integer", minimum: 1, maximum: 60, default: 20 },
  }, required: ["actionId", "deviceIds"] },
  outputSchema: { type: "object", additionalProperties: false, properties: {
    actionId: { type: "string" }, action: { type: "string" }, results: { type: "array", items: { type: "object", additionalProperties: false, properties: {
      processId: { type: "string" }, status: { type: "string", enum: ["exited", "running", "lost"] }, output: { type: "string" },
      exitCode: { type: ["integer", "null"] }, signal: { type: ["string", "null"] }, error: { type: ["string", "null"] }, nextOffset: { type: "integer" },
      outputTruncated: { type: "boolean" },
    }, required: ["processId", "status", "output", "exitCode", "signal", "error", "nextOffset", "outputTruncated"] } },
  }, required: ["actionId", "action", "results"] },
  annotations: { readOnlyHint: false, destructiveHint: true, idempotentHint: false, openWorldHint: true },
  async run(context, args, params) {
    const actionId = String(args.actionId || ""), deviceIds = [...new Set(Array.isArray(args.deviceIds) ? args.deviceIds.map(String) : [])].sort();
    const action = q<any>("SELECT id,workspace_id,name,command,cwd,confirm FROM actions WHERE id=?").get(actionId);
    const expectedHash = allowedActions(context).get(actionId);
    if (!action || !expectedHash || expectedHash !== actionHash(action.command, action.cwd)) return complete({ error: "Action is not in this MCP grant or changed after authorization." }, "Action is not in this MCP grant or changed after authorization.", true);
    const allowedDevices = new Set(context.payload.deviceIds);
    const valid = q<{ id: string }>(`SELECT id FROM devices WHERE workspace_id=? AND id IN (${deviceIds.map(() => "?").join(",") || "''"})`)
      .all(action.workspace_id, ...deviceIds).map(row => row.id);
    if (!deviceIds.length || valid.length !== deviceIds.length || deviceIds.some(id => !allowedDevices.has(id))) return complete({ error: "One or more machines are outside this Action's workspace or MCP grant." }, "One or more machines are outside this Action's workspace or MCP grant.", true);
    if (action.confirm) {
      const required = confirmation(context, actionId, deviceIds, params); if (required) return required;
    }
    const results = await Promise.all(deviceIds.map(deviceId => runMcpProcess(context, { deviceId, command: action.command, cwd: action.cwd || "",
      kind: "action", actionId, timeoutSeconds: Number(args.timeoutSeconds || 20) })));
    const summary = results.map(result => result.status === "exited" ? `${result.processId}: exit ${result.exitCode ?? "unknown"}`
      : `${result.processId}: ${result.status}`).join("; ");
    return complete({ actionId, action: action.name, results }, `${action.name}: ${summary}`);
  },
};
