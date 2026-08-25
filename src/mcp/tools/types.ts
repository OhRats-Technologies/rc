import type { McpScope, McpToolContext } from "../types";

export type McpTool = {
  name: string;
  title: string;
  description: string;
  scope: McpScope;
  inputSchema: Record<string, unknown>;
  outputSchema?: Record<string, unknown>;
  annotations?: {
    readOnlyHint?: boolean;
    destructiveHint?: boolean;
    idempotentHint?: boolean;
    openWorldHint?: boolean;
  };
  run: (context: McpToolContext, args: Record<string, unknown>, params: Record<string, unknown>) => unknown | Promise<unknown>;
};

export function complete(value: unknown, text: string, isError = false) {
  return { resultType: "complete", content: [{ type: "text", text }], structuredContent: value,
    ...(isError ? { isError: true } : {}) };
}

export function hasScope(scopes: string[], required: McpScope) {
  if (required === "mcp:observe") return scopes.some(scope => scope.startsWith("mcp:"));
  return scopes.includes(required);
}
