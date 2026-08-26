import { afterAll, beforeAll, describe, expect, test } from "bun:test";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

let dataDir = "";
let app: typeof import("./app").app;
let q: typeof import("./db").q;
let createAgentChallenge: typeof import("./gateway").createAgentChallenge;
let verifyAgent: typeof import("./gateway").verifyAgent;
let agentSocketHandlers: typeof import("./gateway").agentSocketHandlers;
let checkOrigin: typeof import("./http-utils").checkOrigin;
let consumeStepUp: typeof import("./step-up").consumeStepUp;
let consumeStepUpOrRecentSession: typeof import("./step-up").consumeStepUpOrRecentSession;
let controlAuthorizationOptions: typeof import("./control-auth").controlAuthorizationOptions;
let controlGrantChallenge: typeof import("./control-auth").controlGrantChallenge;
let createOAuthRequest: typeof import("./mcp/oauth").createOAuthRequest;
let denyOAuthRequest: typeof import("./mcp/oauth").denyOAuthRequest;
let restartOAuthRequest: typeof import("./mcp/oauth").restartOAuthRequest;
let sha: typeof import("./db").sha;
let sshPrincipalForDevice: typeof import("./ssh-keys").sshPrincipalForDevice;

beforeAll(async () => {
  dataDir = await mkdtemp(join(tmpdir(), "rc-security-test-"));
  Bun.env.DATA_DIR = dataDir;
  Bun.env.PUBLIC_URL = "http://localhost:3000";
  ({ q, sha } = await import("./db"));
  ({ createAgentChallenge, verifyAgent, agentSocketHandlers } = await import("./gateway"));
  ({ checkOrigin } = await import("./http-utils"));
  ({ consumeStepUp, consumeStepUpOrRecentSession } = await import("./step-up"));
  ({ controlAuthorizationOptions, controlGrantChallenge } = await import("./control-auth"));
  ({ createOAuthRequest, denyOAuthRequest, restartOAuthRequest } = await import("./mcp/oauth"));
  ({ sshPrincipalForDevice } = await import("./ssh-keys"));
  ({ app } = await import("./app"));
});

afterAll(async () => { if (dataDir) await rm(dataDir, { recursive: true, force: true }); });

async function enrolledDevice() {
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

describe("agent authentication", () => {
  test("a signed challenge is accepted exactly once", async () => {
    const { deviceId, privateKey } = await enrolledDevice();
    const challenge = createAgentChallenge(deviceId)!;
    const path = "/api/v1/agent/self";
    const payload = `rc-auth-v2\n${deviceId}\n${challenge.challenge}\nGET\n${path}`;
    const signature = new Uint8Array(await crypto.subtle.sign("Ed25519", privateKey, new TextEncoder().encode(payload)));
    const encoded = Buffer.from(signature).toString("base64url");
    const request = () => new Request(`http://localhost:3000${path}?device=${deviceId}`, {
      headers: { "x-rc-challenge": challenge.challenge, "x-rc-signature": encoded },
    });
    expect(await verifyAgent(request(), deviceId)).toBe(deviceId);
    expect(await verifyAgent(request(), deviceId)).toBeNull();
  });
});

describe("HTTP hardening", () => {
  test("responses carry restrictive security headers", async () => {
    const response = await app.handle(new Request("http://localhost:3000/healthz"));
    expect(response.headers.get("content-security-policy")).toContain("frame-ancestors 'none'");
    expect(response.headers.get("referrer-policy")).toBe("no-referrer");
    expect(response.headers.get("x-content-type-options")).toBe("nosniff");
  });

  test("sensitive auth routes are rate limited", async () => {
    let response = new Response();
    for (let index = 0; index < 31; index++) {
      response = await app.handle(new Request("http://localhost:3000/api/v1/auth/login/options", {
        method: "POST", headers: { "content-type": "application/json", "x-forwarded-for": "198.51.100.40" }, body: "{}",
      }));
    }
    expect(response.status).toBe(429);
    expect(Number(response.headers.get("retry-after"))).toBeGreaterThan(0);
  });

  test("cookie-authenticated mutations require browser same-origin evidence", () => {
    const base = { method: "POST", headers: { cookie: "rc_session=test" } } as RequestInit;
    expect(checkOrigin(new Request("http://localhost:3000/account/name", base))).toBe(false);
    expect(checkOrigin(new Request("http://localhost:3000/account/name", {
      ...base, headers: { cookie: "rc_session=test", "sec-fetch-site": "cross-site" },
    }))).toBe(false);
    expect(checkOrigin(new Request("http://localhost:3000/account/name", {
      ...base, headers: { cookie: "rc_session=test", "sec-fetch-site": "same-origin" },
    }))).toBe(true);
    expect(checkOrigin(new Request("http://localhost:3000/account/name", {
      ...base, headers: { cookie: "rc_session=test", origin: "http://localhost:3000" },
    }))).toBe(true);
    expect(checkOrigin(new Request("http://localhost:3000/devices/enroll", {
      ...base, headers: { cookie: "rc_session=test", origin: "null", "sec-fetch-site": "same-origin",
        "sec-fetch-mode": "navigate", "sec-fetch-dest": "document" },
    }))).toBe(true);
    expect(checkOrigin(new Request("http://localhost:3000/devices/enroll", {
      ...base, headers: { cookie: "rc_session=test", origin: "null", "sec-fetch-site": "cross-site",
        "sec-fetch-mode": "navigate", "sec-fetch-dest": "document" },
    }))).toBe(false);
    expect(checkOrigin(new Request("http://localhost:3000/devices/enroll", {
      ...base, headers: { cookie: "rc_session=test", origin: "https://evil.example", "sec-fetch-site": "same-origin",
        "sec-fetch-mode": "navigate", "sec-fetch-dest": "document" },
    }))).toBe(false);
  });

  test("requests cannot mix bearer and proof-of-possession identities", async () => {
    const response = await app.handle(new Request("http://localhost:3000/api/v1/workspaces", {
      headers: { authorization: "Bearer rc_cli_invalid", "x-rc-key-id": "also-invalid" },
    }));
    expect(response.status).toBe(400);
  });
});

describe("API key scopes", () => {
  test("read-only keys cannot mutate workspaces", async () => {
    const userId = crypto.randomUUID(), keyId = crypto.randomUUID(), t = Date.now();
    const pair = await crypto.subtle.generateKey({ name: "Ed25519" }, true, ["sign", "verify"]);
    const publicKey = Buffer.from(await crypto.subtle.exportKey("raw", pair.publicKey)).toString("base64url");
    q("INSERT INTO users(id,name,created_at) VALUES(?,?,?)").run(userId, "Scoped User", t);
    q("INSERT INTO api_tokens(id,user_id,name,token_hash,public_key,scopes,created_at) VALUES(?,?,?,?,?,?,?)")
      .run(keyId, userId, "Read only", `pop:${keyId}`, publicKey, '["read"]', t);

    async function signed(method: string, path: string, body = "") {
      const timestamp = String(Math.floor(Date.now() / 1000)), nonce = crypto.randomUUID();
      const digest = new Bun.CryptoHasher("sha256").update(body).digest("hex");
      const payload = `rc-api-v1\n${keyId}\n${timestamp}\n${nonce}\n${method}\n${path}\n${digest}`;
      const signature = Buffer.from(await crypto.subtle.sign("Ed25519", pair.privateKey, new TextEncoder().encode(payload))).toString("base64url");
      return new Request(`http://localhost:3000${path}`, { method, body: body || undefined, headers: {
        "content-type": "application/json", "x-rc-key-id": keyId, "x-rc-timestamp": timestamp,
        "x-rc-nonce": nonce, "x-rc-signature": signature,
      } });
    }

    const list = await app.handle(await signed("GET", "/api/v1/workspaces"));
    expect(list.status).toBe(200);
    const create = await app.handle(await signed("POST", "/api/v1/workspaces", JSON.stringify({ name: "Denied" })));
    expect(create.status).toBe(403);
  });

  test("proof-of-possession request nonces cannot be replayed", async () => {
    const userId = crypto.randomUUID(), keyId = crypto.randomUUID(), t = Date.now();
    const pair = await crypto.subtle.generateKey({ name: "Ed25519" }, true, ["sign", "verify"]);
    const publicKey = Buffer.from(await crypto.subtle.exportKey("raw", pair.publicKey)).toString("base64url");
    q("INSERT INTO users(id,name,created_at) VALUES(?,?,?)").run(userId, "Replay User", t);
    q("INSERT INTO api_tokens(id,user_id,name,token_hash,public_key,scopes,created_at) VALUES(?,?,?,?,?,?,?)")
      .run(keyId, userId, "Replay test", `pop:${keyId}`, publicKey, '["read"]', t);
    const path = "/api/v1/workspaces", timestamp = String(Math.floor(Date.now() / 1000)), nonce = crypto.randomUUID();
    const digest = new Bun.CryptoHasher("sha256").update("").digest("hex");
    const payload = `rc-api-v1\n${keyId}\n${timestamp}\n${nonce}\nGET\n${path}\n${digest}`;
    const signature = Buffer.from(await crypto.subtle.sign("Ed25519", pair.privateKey, new TextEncoder().encode(payload))).toString("base64url");
    const request = () => new Request(`http://localhost:3000${path}`, { headers: {
      "x-rc-key-id": keyId, "x-rc-timestamp": timestamp, "x-rc-nonce": nonce, "x-rc-signature": signature,
    } });
    expect((await app.handle(request())).status).toBe(200);
    expect((await app.handle(request())).status).toBe(401);
  });
});

describe("SSH gateway authorization", () => {
  test("routes by immutable device ID and current role", async () => {
    const { userId, deviceId } = await enrolledDevice(), controlId = crypto.randomUUID(), keyId = crypto.randomUUID(), t = Date.now();
    q(`INSERT INTO control_clients(id,user_id,signing_public_key,credential_id,grant,assertion,created_at,expires_at,last_used) VALUES(?,?,?,?,?,?,?,?,NULL)`).run(controlId,userId,"public","credential","grant","assertion",t,0);
    q(`INSERT INTO ssh_keys(id,user_id,name,algorithm,key_data,public_key,control_client_id,created_at,last_used) VALUES(?,?,?,?,?,?,?,?,NULL)`).run(keyId,userId,"Laptop","ssh-ed25519",crypto.randomUUID(),"ssh-ed25519 blob",controlId,t);
    expect(sshPrincipalForDevice(keyId, deviceId)?.device_id).toBe(deviceId);
    q("UPDATE devices SET name='Renamed' WHERE id=?").run(deviceId); expect(sshPrincipalForDevice(keyId, deviceId)?.device_id).toBe(deviceId);
    expect(sshPrincipalForDevice(keyId, "Renamed")).toBeNull();
    q("UPDATE workspace_members SET role='viewer' WHERE user_id=?").run(userId); expect(sshPrincipalForDevice(keyId, deviceId)).toBeNull();
  });
});

describe("fresh passkey step-up", () => {
  test("step-up tokens are user-bound and consumed exactly once", () => {
    const userId = crypto.randomUUID(), otherId = crypto.randomUUID(), token = `step_${crypto.randomUUID()}`, t = Date.now();
    q("INSERT INTO users(id,name,created_at) VALUES(?,?,?)").run(userId, "Step User", t);
    q("INSERT INTO users(id,name,created_at) VALUES(?,?,?)").run(otherId, "Other User", t);
    q("INSERT INTO step_up_tokens(token_hash,user_id,created_at,expires_at) VALUES(?,?,?,?)").run(sha(token), userId, t, t + 120_000);
    const request = () => new Request("http://localhost:3000/api/v1/tokens", { method: "POST", headers: { "x-rc-step-up": token } });
    expect(() => consumeStepUp(request(), { id: otherId, name: "Other User" })).toThrow("fresh passkey verification required");
    expect(() => consumeStepUp(request(), { id: userId, name: "Step User" })).not.toThrow();
    expect(() => consumeStepUp(request(), { id: userId, name: "Step User" })).toThrow("fresh passkey verification required");
  });

  test("recent WebAuthn browser sessions can add a passkey but stale sessions still need step-up", () => {
    const userId = crypto.randomUUID(), fresh = `sess_${crypto.randomUUID()}`, stale = `sess_${crypto.randomUUID()}`, t = Date.now();
    q("INSERT INTO users(id,name,created_at) VALUES(?,?,?)").run(userId, "Passkey User", t);
    q("INSERT INTO auth_sessions(token_hash,user_id,created_at,expires_at) VALUES(?,?,?,?)").run(sha(fresh), userId, t, t + 60_000);
    q("INSERT INTO auth_sessions(token_hash,user_id,created_at,expires_at) VALUES(?,?,?,?)").run(sha(stale), userId, t - 10 * 60_000, t + 60_000);
    const user = { id: userId, name: "Passkey User" };
    const request = (token: string) => new Request("http://localhost:3000/api/v1/passkeys/options", {
      method: "POST", headers: { cookie: `rc_session=${encodeURIComponent(token)}` },
    });
    expect(() => consumeStepUpOrRecentSession(request(fresh), user)).not.toThrow();
    expect(() => consumeStepUpOrRecentSession(request(stale), user)).toThrow("fresh passkey verification required");
  });

  test("control authorization stores the exact challenge sent to the browser", async () => {
    const userId = crypto.randomUUID(), t = Date.now();
    q("INSERT INTO users(id,name,created_at) VALUES(?,?,?)").run(userId, "Control User", t);
    const start = await controlAuthorizationOptions({ id: userId, name: "Control User" }, {
      clientId: crypto.randomUUID(), signingPublicKey: Buffer.alloc(32, 7).toString("base64url"),
    });
    const row = q<{ challenge: string }>("SELECT challenge FROM control_authorizations WHERE id=?").get(start.authorizationId)!;
    expect(start.options.challenge).toBe(row.challenge);
    expect(start.options.challenge).toBe(controlGrantChallenge(start.grant));
  });
});

function mcpRpc(method: string, id = 1, params?: Record<string, unknown>, token = "") {
  const headers: Record<string, string> = { "content-type": "application/json", "mcp-protocol-version": "2026-07-28", "mcp-method": method };
  if (method === "tools/call") headers["mcp-name"] = String(params?.name || "");
  if (token) headers.authorization = `Bearer ${token}`;
  return new Request("http://localhost:3000/mcp", { method: "POST", headers,
    body: JSON.stringify({ jsonrpc: "2.0", id, method, ...(params ? { params } : {}) }) });
}

function seededMcpCode(scopes: string[]) {
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

describe("MCP transport and OAuth", () => {
  test("consent cancellation preserves state and account switching restarts the same OAuth request", () => {
    const userId = crypto.randomUUID(), clientId = `mcp_client_${crypto.randomUUID()}`, t = Date.now();
    const redirect = "http://127.0.0.1:49153/callback", challenge = "b".repeat(43), state = "oauth-state";
    q("INSERT INTO users(id,name,created_at) VALUES(?,?,?)").run(userId, "Consent User", t);
    q("INSERT INTO mcp_clients(id,name,redirect_uris,created_at) VALUES(?,?,?,?)").run(clientId, "Consent Test", JSON.stringify([redirect]), t);
    const url = new URL("http://localhost:3000/oauth/authorize");
    Object.entries({ response_type: "code", client_id: clientId, redirect_uri: redirect, scope: "mcp:observe mcp:terminal",
      state, code_challenge: challenge, code_challenge_method: "S256", resource: "http://localhost:3000/mcp" })
      .forEach(([key, value]) => url.searchParams.set(key, value));
    const user = { id: userId, name: "Consent User" };
    const cancelled = createOAuthRequest(user, url);
    const denied = new URL(denyOAuthRequest(userId, cancelled.requestId));
    expect(denied.searchParams.get("error")).toBe("access_denied"); expect(denied.searchParams.get("state")).toBe(state);
    const restarted = createOAuthRequest(user, url), next = new URL(restartOAuthRequest(userId, restarted.requestId), "http://localhost:3000");
    expect(next.searchParams.get("client_id")).toBe(clientId); expect(next.searchParams.get("code_challenge")).toBe(challenge);
    expect(next.searchParams.get("resource")).toBe("http://localhost:3000/mcp");
  });

  test("publishes discovery, challenges protected calls, and rejects mismatched method headers", async () => {
    const metadata = await app.handle(new Request("http://localhost:3000/.well-known/oauth-protected-resource"));
    expect((await metadata.json() as any).resource).toBe("http://localhost:3000/mcp");
    const denied = await app.handle(mcpRpc("tools/list"));
    expect(denied.status).toBe(401); expect(denied.headers.get("www-authenticate")).toContain("mcp:observe");
    const mismatched = mcpRpc("server/discover"); mismatched.headers.set("mcp-method", "tools/list");
    expect((await app.handle(mismatched)).status).toBe(400);
  });

  test("PKCE codes and refresh tokens are one-time and scopes filter tools", async () => {
    const seeded = seededMcpCode(["mcp:observe"]);
    const form = new URLSearchParams({ grant_type: "authorization_code", client_id: seeded.clientId, code: seeded.code,
      redirect_uri: seeded.redirect, code_verifier: seeded.verifier, resource: "http://localhost:3000/mcp" });
    const token = await app.handle(new Request("http://localhost:3000/oauth/token", { method: "POST", body: form }));
    expect(token.status).toBe(200);
    const credentials = await token.json() as any;
    expect(credentials.access_token).toStartWith("mcp_access_"); expect(credentials.refresh_token).toStartWith("mcp_refresh_");
    expect((await app.handle(new Request("http://localhost:3000/oauth/token", { method: "POST", body: form }))).status).toBe(400);
    const listed = await app.handle(mcpRpc("tools/list", 2, undefined, credentials.access_token));
    const descriptors = (await listed.json() as any).result.tools, names = descriptors.map((tool: any) => tool.name);
    expect(names).toContain("machines_list"); expect(names).toContain("process_status"); expect(names).not.toContain("process_run");
    const machines = descriptors.find((tool: any) => tool.name === "machines_list");
    expect(machines.annotations.readOnlyHint).toBe(true); expect(machines.outputSchema.properties.machines.type).toBe("array");
    const terminal = await app.handle(mcpRpc("tools/call", 3, { name: "process_run", arguments: { deviceId: "x", command: "id" } }, credentials.access_token));
    expect(terminal.status).toBe(403); expect(terminal.headers.get("www-authenticate")).toContain("mcp:terminal");
    const refresh = new URLSearchParams({ grant_type: "refresh_token", client_id: seeded.clientId,
      refresh_token: credentials.refresh_token, resource: "http://localhost:3000/mcp" });
    expect((await app.handle(new Request("http://localhost:3000/oauth/token", { method: "POST", body: refresh }))).status).toBe(200);
    expect((await app.handle(new Request("http://localhost:3000/oauth/token", { method: "POST", body: refresh }))).status).toBe(400);
  });

  test("MCP process output is relayed without being retained in SQLite", async () => {
    const { userId, deviceId } = await enrolledDevice(), processId = crypto.randomUUID(), t = Date.now();
    q(`INSERT INTO processes(id,device_id,command,cwd,status,encrypted,mcp,cols,rows,created_by,created_at)
      VALUES(?,?,?,NULL,'running',1,1,80,24,?,?)`).run(processId, deviceId, "[mcp]", userId, t);
    agentSocketHandlers.message(deviceId, { type: "process.stdout", id: processId, data: Buffer.from("mcp-secret-output").toString("base64url") });
    const row = q<any>("SELECT output_head,output_tail,output_chars FROM processes WHERE id=?").get(processId);
    expect(row.output_head).toBe(""); expect(row.output_tail).toBe(""); expect(row.output_chars).toBe(0);
  });
});
