export const MCP_PROTOCOL_VERSION = "2026-07-28";
export const MCP_SCOPES = ["mcp:observe", "mcp:actions", "mcp:terminal"] as const;
export type McpScope = typeof MCP_SCOPES[number];

export type McpActionGrant = { id: string; hash: string };
export type McpGrantPayload = {
  v: 1;
  id: string;
  userId: string;
  clientId: string;
  clientName: string;
  deviceIds: string[];
  scopes: McpScope[];
  actions: McpActionGrant[];
  issuedAt: number;
  expiresAt: number;
};

export type McpGrantRecord = {
  id: string;
  user_id: string;
  client_id: string;
  name: string;
  grant: string;
  control_client_id: string;
  grant_signature: string;
  credential_id: string;
  control_grant: string;
  control_assertion: string;
  created_at: number;
  expires_at: number;
  last_used: number | null;
  revoked_at: number | null;
};

export type McpToolContext = { grant: McpGrantRecord; payload: McpGrantPayload };
export type JsonRpcRequest = {
  jsonrpc: "2.0";
  id?: string | number | null;
  method: string;
  params?: Record<string, unknown>;
};
