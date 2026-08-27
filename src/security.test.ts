import { afterAll, beforeAll, describe, expect, test } from "bun:test";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { enrolledDevice, mcpRpc, seededMcpCode } from "./security-test-fixtures";

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
let removeDevice: typeof import("./devices").removeDevice;

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
  ({ removeDevice } = await import("./devices"));
  ({ app } = await import("./app"));
});

afterAll(async () => { if (dataDir) await rm(dataDir, { recursive: true, force: true }); });

describe("agent authentication", () => {
  test("a signed challenge is accepted exactly once", async () => {
    const { deviceId, privateKey } = await enrolledDevice(q);
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

  test("deleted devices keep only a revocation identity for signed reconciliation", async () => {
    const { userId, deviceId, privateKey } = await enrolledDevice(q);
    removeDevice({ id: userId, name: "Security Test" }, deviceId);
    expect(q("SELECT 1 FROM devices WHERE id=?").get(deviceId)).toBeNull();
    expect(q("SELECT public_key FROM revoked_devices WHERE id=?").get(deviceId)).toBeTruthy();

    const challenge = createAgentChallenge(deviceId)!;
    const path = "/api/v1/agent/ws";
    const payload = `rc-auth-v2\n${deviceId}\n${challenge.challenge}\nGET\n${path}`;
    const signature = new Uint8Array(await crypto.subtle.sign("Ed25519", privateKey, new TextEncoder().encode(payload)));
    const request = new Request(`http://localhost:3000${path}?device=${deviceId}`, {
      headers: { "x-rc-challenge": challenge.challenge, "x-rc-signature": Buffer.from(signature).toString("base64url") },
    });
    expect(await verifyAgent(request, deviceId)).toBe(deviceId);
    expect(q("SELECT 1 FROM revoked_devices WHERE id=?").get(deviceId)).toBeTruthy();

    const next = createAgentChallenge(deviceId)!;
    const nextPayload = `rc-auth-v2\n${deviceId}\n${next.challenge}\nGET\n/api/v1/agent/self`;
    const nextSignature = new Uint8Array(await crypto.subtle.sign("Ed25519", privateKey, new TextEncoder().encode(nextPayload)));
    const response = await app.handle(new Request(`http://localhost:3000/api/v1/agent/self?device=${deviceId}`, {
      headers: { "x-rc-challenge": next.challenge, "x-rc-signature": Buffer.from(nextSignature).toString("base64url") },
    }));
    expect(response.status).toBe(410);
  });
});

describe("HTTP hardening", () => {
  test("responses carry restrictive security headers", async () => {
    const response = await app.handle(new Request("http://localhost:3000/healthz"));
    const csp = response.headers.get("content-security-policy") || "";
    expect(csp).toContain("frame-ancestors 'none'");
    expect(csp).toContain("style-src 'self' 'unsafe-inline'");
    expect(csp.match(/script-src[^;]*/)?.[0]).not.toContain("'unsafe-inline'");
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
    const { userId, deviceId } = await enrolledDevice(q), controlId = crypto.randomUUID(), keyId = crypto.randomUUID(), t = Date.now();
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
    const seeded = seededMcpCode(q, sha, ["mcp:observe"]);
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

  test("process sync marks hosted rows missing from the Node as lost", async () => {
    const { userId, deviceId } = await enrolledDevice(q), kept = crypto.randomUUID(), stale = crypto.randomUUID(), t = Date.now();
    for (const processId of [kept, stale]) q(`INSERT INTO processes(id,device_id,origin,status,terminal,created_by,created_at,started_at) VALUES(?,?,'control','running',1,?,?,?)`).run(processId, deviceId, userId, t, t);
    agentSocketHandlers.message(deviceId, { type: "process.sync", ids: [kept] });
    expect(q<any>("SELECT status FROM processes WHERE id=?").get(kept).status).toBe("running");
    const row = q<any>("SELECT status,error,completed_at FROM processes WHERE id=?").get(stale);
    expect([row.status, row.error]).toEqual(["lost", "RC Node reconnected without this process"]); expect(row.completed_at).toBeGreaterThan(0);
  });

  test("process rows contain lifecycle metadata only", async () => {
    const { userId, deviceId } = await enrolledDevice(q), processId = crypto.randomUUID(), t = Date.now();
    q(`INSERT INTO processes(id,device_id,origin,status,terminal,created_by,created_at) VALUES(?,?,'mcp','running',0,?,?)`).run(processId, deviceId, userId, t);
    agentSocketHandlers.message(deviceId, { type: "process.stdout", id: processId, data: Buffer.from("mcp-secret-output").toString("base64url") });
    const columns = q<{ name: string }>("PRAGMA table_info(processes)").all().map(column => column.name);
    for (const removed of ["command", "cwd", "encrypted", "mcp", "output_head", "output_tail", "output_chars", "revision", "cols", "rows"]) expect(columns).not.toContain(removed);
    expect(q<any>("SELECT id,origin,status,terminal FROM processes WHERE id=?").get(processId)).toEqual({ id: processId, origin: "mcp", status: "running", terminal: 0 });
  });
});
