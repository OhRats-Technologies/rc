import { afterAll, beforeAll, describe, expect, test } from "bun:test";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

let dataDir = "";
let app: typeof import("./app").app;
let q: typeof import("./db").q;
let sha: typeof import("./db").sha;
let createAgentChallenge: typeof import("./gateway").createAgentChallenge;
let verifyAgent: typeof import("./gateway").verifyAgent;

beforeAll(async () => {
  dataDir = await mkdtemp(join(tmpdir(), "rc-security-test-"));
  Bun.env.DATA_DIR = dataDir;
  Bun.env.PUBLIC_URL = "http://localhost:3000";
  ({ q, sha } = await import("./db"));
  ({ createAgentChallenge, verifyAgent } = await import("./gateway"));
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
});

describe("API key scopes", () => {
  test("read-only keys cannot mutate workspaces", async () => {
    const userId = crypto.randomUUID(), t = Date.now(), token = "rc_api_test_scope";
    q("INSERT INTO users(id,name,created_at) VALUES(?,?,?)").run(userId, "Scoped User", t);
    q("INSERT INTO api_tokens(id,user_id,name,token_hash,scopes,created_at) VALUES(?,?,?,?,?,?)")
      .run(crypto.randomUUID(), userId, "Read only", sha(token), '["read"]', t);
    const headers = { authorization: `Bearer ${token}`, "content-type": "application/json" };
    const list = await app.handle(new Request("http://localhost:3000/api/v1/workspaces", { headers }));
    expect(list.status).toBe(200);
    const create = await app.handle(new Request("http://localhost:3000/api/v1/workspaces", {
      method: "POST", headers, body: JSON.stringify({ name: "Denied" }),
    }));
    expect(create.status).toBe(403);
  });
});
