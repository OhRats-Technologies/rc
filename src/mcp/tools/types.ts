import type { McpScope, McpToolContext } from "../types";

export type McpTool = {
  name: string;
  title: string;
  description: string;
  scope: McpScope;
  inputSchema: Record<string, unknown>;
  run: (context: McpToolContext, args: Record<string, unknown>, params: Record<string, unknown>) => unknown | Promise<unknown>;
};

export function complete(value: unknown, isError = false) {
  return { resultType: "complete", content: [{ type: "text", text: typeof value === "string" ? value : JSON.stringify(value) }],
    structuredContent: value, ...(isError ? { isError: true } : {}) };
}

export function hasScope(scopes: string[], required: McpScope) {
  if (required === "mcp:observe") return scopes.some(scope => scope.startsWith("mcp:"));
  return scopes.includes(required);
}
