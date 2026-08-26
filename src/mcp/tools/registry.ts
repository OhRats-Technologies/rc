import type { McpToolContext } from "../types";
import { machinesListTool } from "./machines";
import { processStatusTool } from "./process-status";
import { terminalRunTool } from "./terminal";
import { hasScope, type McpTool } from "./types";

const tools: McpTool[] = [machinesListTool, terminalRunTool, processStatusTool];

export function toolsFor(context: McpToolContext) { return tools.filter(tool => hasScope(context.payload.scopes, tool.scope)); }
export function registeredTool(name: string) { return tools.find(tool => tool.name === name) || null; }
export function toolFor(context: McpToolContext, name: string) {
  const tool = registeredTool(name); return tool && hasScope(context.payload.scopes, tool.scope) ? tool : null;
}
export function toolDescriptor(tool: McpTool) {
  return { name: tool.name, title: tool.title, description: tool.description, inputSchema: tool.inputSchema,
    ...(tool.outputSchema ? { outputSchema: tool.outputSchema } : {}), ...(tool.annotations ? { annotations: tool.annotations } : {}) };
}
