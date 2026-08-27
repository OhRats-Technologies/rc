type Query = typeof import("./db").q;

export async function enrolledDevice(q: Query) {
  const userId = crypto.randomUUID(), workspaceId = crypto.randomUUID(), deviceId = crypto.randomUUID(), t = Date.now();
  const pair = await crypto.subtle.generateKey({ name: "Ed25519" }, true, ["sign", "verify"]);
  const spki = new Uint8Array(await crypto.subtle.exportKey("spki", pair.publicKey));
  const body = Buffer.from(spki).toString("base64").match(/.{1,64}/g)?.join("\n") || "";
  const publicKey = `-----BEGIN PUBLIC KEY-----\n${body}\n-----END PUBLIC KEY-----\n`;
  q("INSERT INTO users(id,name,created_at) VALUES(?,?,?)").run(userId, "Security Test", t);
  q("INSERT INTO workspaces(id,name,created_by,created_at) VALUES(?,?,?,?)").run(workspaceId, "Test", userId, t);
  q("INSERT INTO workspace_members(workspace_id,user_id,role,joined_at) VALUES(?,?,?,?)").run(workspaceId, userId, "owner", t);
  q(`INSERT INTO devices(id,workspace_id,name,hostname,platform,arch,public_key,agent_version,capabilities,last_seen,created_at)
    VALUES(?,?,?,?,?,?,?,?,?,?,?)`).run(deviceId, workspaceId, "Node", "node", "darwin", "arm64", publicKey, "0.7.0", "[]", t, t);
  return { userId, deviceId, privateKey: pair.privateKey };
}

export function mcpRpc(method: string, id = 1, params?: Record<string, unknown>, token = "") {
  const headers: Record<string, string> = { "content-type": "application/json", "mcp-protocol-version": "2026-07-28", "mcp-method": method };
  if (method === "tools/call") headers["mcp-name"] = String(params?.name || "");
  if (token) headers.authorization = `Bearer ${token}`;
  return new Request("http://localhost:3000/mcp", { method: "POST", headers,
    body: JSON.stringify({ jsonrpc: "2.0", id, method, ...(params ? { params } : {}) }) });
}

export function seededMcpCode(q: Query, sha: (value: string) => string, scopes: string[]) {
  const userId = crypto.randomUUID(), clientId = `mcp_client_${crypto.randomUUID()}`, grantId = crypto.randomUUID(), t = Date.now();
  const code = `mcp_code_${crypto.randomUUID()}`, verifier = "a".repeat(43);
  const digest = new Bun.CryptoHasher("sha256").update(verifier).digest();
  const challenge = Buffer.from(digest).toString("base64url"), redirect = "http://127.0.0.1:49152/callback";
  const grant = JSON.stringify({ v: 1, id: grantId, userId, clientId, clientName: "MCP Test", deviceIds: [], scopes,
    issuedAt: t, expiresAt: t + 60 * 60_000 });
  q("INSERT INTO users(id,name,created_at) VALUES(?,?,?)").run(userId, "MCP User", t);
  q("INSERT INTO mcp_clients(id,name,redirect_uris,created_at) VALUES(?,?,?,?)").run(clientId, "MCP Test", JSON.stringify([redirect]), t);
  q(`INSERT INTO mcp_grants(id,user_id,client_id,name,grant,control_client_id,grant_signature,credential_id,control_grant,control_assertion,created_at,expires_at)
    VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`).run(grantId, userId, clientId, "MCP Test", grant, "control", "sig", "credential", "grant", "assertion", t, t + 60 * 60_000);
  q("INSERT INTO mcp_codes(code_hash,grant_id,redirect_uri,code_challenge,resource,expires_at) VALUES(?,?,?,?,?,?)")
    .run(sha(code), grantId, redirect, challenge, "http://localhost:3000/mcp", t + 60_000);
  return { clientId, code, verifier, redirect };
}
