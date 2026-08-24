import { canWrite, deviceRole, logEvent, User } from "./core";
import { db, id, now, q, sha } from "./db";
import { disconnectDevice, isOnline, verifyAgent } from "./gateway";
import { body, fail, json } from "./http-utils";

export async function handleAgentEnroll(req: Request, path: string): Promise<Response | null> {
  if (path !== "/api/v1/agent/enroll" || req.method !== "POST") return null;
  const input = await body(req), token = String(input.token || "").trim();
  const enrollment = q<any>(`SELECT * FROM enrollment_tokens
    WHERE token_hash=? AND used_at IS NULL AND expires_at>?`).get(sha(token), now());
  if (!enrollment) return fail("invalid or expired enrollment token", 401);
  const publicKey = String(input.publicKey || "");
  if (!publicKey.includes("BEGIN PUBLIC KEY")) return fail("invalid public key");
  if (q("SELECT id FROM devices WHERE public_key=?").get(publicKey)) return fail("device key already enrolled", 409);
  const deviceId = id(), t = now();
  db.transaction(() => {
    q(`INSERT INTO devices(id,workspace_id,name,hostname,platform,arch,public_key,agent_version,capabilities,last_seen,created_at)
      VALUES(?,?,?,?,?,?,?,?,?,?,?)`).run(
      deviceId, enrollment.workspace_id, String(input.name || input.hostname || "Device").slice(0, 120),
      String(input.hostname || "unknown").slice(0, 255), String(input.platform || "unknown").slice(0, 40),
      String(input.arch || "unknown").slice(0, 40), publicKey, String(input.agentVersion || "unknown").slice(0, 40),
      JSON.stringify(Array.isArray(input.capabilities) ? input.capabilities : []), t, t,
    );
    q("UPDATE enrollment_tokens SET used_at=? WHERE id=?").run(t, enrollment.id);
  })();
  logEvent("device.enrolled", enrollment.workspace_id, enrollment.created_by, deviceId);
  return json({ deviceId }, 201);
}

export async function handleAgentUnregister(req: Request, url: URL): Promise<Response | null> {
  if (url.pathname !== "/api/v1/agent/self" || !["GET", "DELETE"].includes(req.method)) return null;
  const requestedDevice = url.searchParams.get("device") || "";
  if (!requestedDevice || !q("SELECT 1 FROM devices WHERE id=?").get(requestedDevice)) return fail("device not found", 404);
  const deviceId = await verifyAgent(url);
  if (!deviceId) return fail("invalid agent signature", 401);
  const device = q<any>("SELECT workspace_id,name,agent_version FROM devices WHERE id=?").get(deviceId);
  if (!device) return fail("device not found", 404);
  if (req.method === "GET") return json({ name: device.name, online: isOnline(deviceId), agentVersion: device.agent_version });
  disconnectDevice(deviceId);
  q("DELETE FROM devices WHERE id=?").run(deviceId);
  logEvent("device.unenrolled", device.workspace_id, null, null, { deviceId });
  return json({ ok: true });
}

function listDevices(user: User) {
  return q<any>(`SELECT d.id,d.workspace_id,d.name,d.hostname,d.platform,d.arch,d.agent_version,d.capabilities,
    d.last_seen,d.created_at,w.name workspace_name FROM devices d JOIN workspaces w ON w.id=d.workspace_id
    JOIN workspace_members wm ON wm.workspace_id=d.workspace_id WHERE wm.user_id=? ORDER BY d.name`).all(user.id)
    .map((row: any) => ({ ...row, online: isOnline(row.id), capabilities: JSON.parse(row.capabilities || "[]") }));
}

function deviceDetail(user: User, deviceId: string) {
  const device = q<any>(`SELECT d.id,d.workspace_id,d.name,d.hostname,d.platform,d.arch,d.agent_version,d.capabilities,
    d.last_seen,d.created_at,w.name workspace_name,wm.role FROM devices d JOIN workspaces w ON w.id=d.workspace_id
    JOIN workspace_members wm ON wm.workspace_id=d.workspace_id WHERE d.id=? AND wm.user_id=?`).get(deviceId, user.id);
  return device ? { ...device, online: isOnline(deviceId), capabilities: JSON.parse(device.capabilities || "[]") } : null;
}

function removeDevice(user: User, deviceId: string) {
  const role = deviceRole(user.id, deviceId);
  if (!role) return fail("device not found", 404);
  if (!canWrite(role)) return fail("forbidden", 403);
  const device = q<any>("SELECT workspace_id,name FROM devices WHERE id=?").get(deviceId);
  if (!device) return fail("device not found", 404);
  disconnectDevice(deviceId, true);
  q("DELETE FROM devices WHERE id=?").run(deviceId);
  logEvent("device.removed", device.workspace_id, user.id, null, { deviceId, name: device.name });
  return json({ ok: true });
}

export async function handleDevices(req: Request, path: string, user: User): Promise<Response | null> {
  if (path === "/api/v1/devices" && req.method === "GET") return json({ devices: listDevices(user) });
  const match = path.match(/^\/api\/v1\/devices\/([^/]+)$/);
  if (match && req.method === "GET") {
    const data = deviceDetail(user, match[1]);
    return data ? json({ device: data }) : fail("device not found", 404);
  }
  if (match && req.method === "DELETE") return removeDevice(user, match[1]);
  return null;
}
