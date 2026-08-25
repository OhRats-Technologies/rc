import { afterAll, beforeAll, describe, expect, test } from "bun:test";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

let dataDir = "";
let app: typeof import("./app").app;
let q: typeof import("./db").q;
let createAgentChallenge: typeof import("./gateway").createAgentChallenge;
let verifyAgent: typeof import("./gateway").verifyAgent;
let checkOrigin: typeof import("./http-utils").checkOrigin;
let runAction: typeof import("./actions").runAction;
let consumeStepUp: typeof import("./step-up").consumeStepUp;
let sha: typeof import("./db").sha;

beforeAll(async () => {
  dataDir = await mkdtemp(join(tmpdir(), "rc-security-test-"));
  Bun.env.DATA_DIR = dataDir;
  Bun.env.PUBLIC_URL = "http://localhost:3000";
  ({ q, sha } = await import("./db"));
  ({ createAgentChallenge, verifyAgent } = await import("./gateway"));
  ({ checkOrigin } = await import("./http-utils"));
  ({ runAction } = await import("./actions"));
  ({ consumeStepUp } = await import("./step-up"));
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

describe("action integrity", () => {
  test("confirmation-required actions cannot be allocated without explicit confirmation", () => {
    const userId = crypto.randomUUID(), workspaceId = crypto.randomUUID(), actionId = crypto.randomUUID(), t = Date.now();
    q("INSERT INTO users(id,name,created_at) VALUES(?,?,?)").run(userId, "Action User", t);
    q("INSERT INTO workspaces(id,name,created_by,created_at) VALUES(?,?,?,?)").run(workspaceId, "Action Workspace", userId, t);
    q("INSERT INTO workspace_members(workspace_id,user_id,role,joined_at) VALUES(?,?,?,?)").run(workspaceId, userId, "owner", t);
    q(`INSERT INTO actions(id,workspace_id,name,description,command,cwd,confirm,created_by,created_at,updated_at)
      VALUES(?,?,?,?,?,?,?,?,?,?)`).run(actionId, workspaceId, "Dangerous", "", "echo guarded", null, 1, userId, t, t);
    expect(() => runAction({ id: userId, name: "Action User" }, actionId, ["missing-device"])).toThrow("explicit confirmation required");
    expect(runAction({ id: userId, name: "Action User" }, actionId, ["missing-device"], true)[0]?.error).toContain("not in this workspace");
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
});
