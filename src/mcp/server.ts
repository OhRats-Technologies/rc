import { PUBLIC_URL, VERSION } from "../config";
import { mcpAccessGrant, MCP_RESOURCE_METADATA } from "./oauth";
import { parseGrant } from "./grants";
import { MCP_PROTOCOL_VERSION, type JsonRpcRequest, type McpToolContext } from "./types";
import { registeredTool, toolDescriptor, toolFor, toolsFor } from "./tools/registry";
import { hasScope } from "./tools/types";

function rpc(id: JsonRpcRequest["id"], result: unknown, status = 200) {
  return Response.json({ jsonrpc: "2.0", id: id ?? null, result }, { status, headers: { "cache-control": "no-store" } });
}

function rpcError(id: JsonRpcRequest["id"], code: number, message: string, status = 200) {
  return Response.json({ jsonrpc: "2.0", id: id ?? null, error: { code, message } }, { status, headers: { "cache-control": "no-store" } });
}

function authError(scope = "mcp:observe mcp:actions", insufficient = false) {
  const value = `Bearer ${insufficient ? 'error="insufficient_scope", ' : ""}resource_metadata="${MCP_RESOURCE_METADATA}", scope="${scope}"`;
  return Response.json({ error: insufficient ? "insufficient_scope" : "unauthorized" }, {
    status: insufficient ? 403 : 401, headers: { "cache-control": "no-store", "www-authenticate": value },
  });
}

function validRequest(value: unknown): value is JsonRpcRequest {
  if (!value || typeof value !== "object") return false;
  const request = value as Partial<JsonRpcRequest>;
  return request.jsonrpc === "2.0" && typeof request.method === "string";
}

function validateHeaders(request: Request, body: JsonRpcRequest) {
  const version = request.headers.get("mcp-protocol-version") || "";
  if (version !== MCP_PROTOCOL_VERSION) return rpcError(body.id, -32022, `Unsupported protocol version: ${version || "missing"}`, 400);
  if (request.headers.get("mcp-method") !== body.method) return rpcError(body.id, -32600, "Mcp-Method header does not match request", 400);
  if (body.method === "tools/call") {
    const name = String(body.params?.name || "");
    if (!name || request.headers.get("mcp-name") !== name) return rpcError(body.id, -32600, "Mcp-Name header does not match tool call", 400);
  }
  return null;
}

function contextFor(request: Request): McpToolContext | null {
  const grant = mcpAccessGrant(request); return grant ? { grant, payload: parseGrant(grant) } : null;
}

export async function handleMcp(request: Request) {
  if (request.method !== "POST") return new Response(null, { status: 405, headers: { allow: "POST", "cache-control": "no-store" } });
  let body: unknown;
  try { body = await request.json(); } catch { return rpcError(null, -32700, "Parse error", 400); }
  if (!validRequest(body)) return rpcError(null, -32600, "Invalid Request", 400);
  const headerError = validateHeaders(request, body); if (headerError) return headerError;

  if (body.method === "server/discover") return rpc(body.id, {
    resultType: "complete", supportedVersions: [MCP_PROTOCOL_VERSION], capabilities: { tools: {} },
    instructions: "Use only the machines and capabilities explicitly granted by the user. Prefer saved Actions over arbitrary terminal commands.",
    ttlMs: 300_000, cacheScope: "public",
    _meta: { "io.modelcontextprotocol/serverInfo": { name: "OhRats RC", version: VERSION, websiteUrl: PUBLIC_URL } },
  });

  const context = contextFor(request); if (!context) return authError();
  if (body.method === "tools/list") return rpc(body.id, {
    resultType: "complete", tools: toolsFor(context).map(toolDescriptor), ttlMs: 30_000, cacheScope: "private",
  });
  if (body.method === "tools/call") {
    const name = String(body.params?.name || ""), registered = registeredTool(name), tool = toolFor(context, name);
    if (!registered) return rpcError(body.id, -32602, `Tool is not available: ${name}`);
    if (!tool || !hasScope(context.payload.scopes, registered.scope)) return authError(registered.scope, true);
    const args = body.params?.arguments;
    try {
      const result = await tool.run(context, args && typeof args === "object" && !Array.isArray(args) ? args as Record<string, unknown> : {}, body.params || {});
      return rpc(body.id, result);
    } catch (error) {
      const message = error instanceof Error ? error.message : "Tool execution failed";
      return rpc(body.id, { resultType: "complete", content: [{ type: "text", text: message }], isError: true });
    }
  }
  return rpcError(body.id, -32601, "Method not found");
}
