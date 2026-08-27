import { deviceRole, logEvent, User } from "./core";
import { db, id, now, q, sha } from "./db";
import { disconnectDevice, isOnline, verifyAgent } from "./gateway";
import { fail, json } from "./http-utils";
import { HttpError } from "./errors";
import { MAX_DEVICES_PER_WORKSPACE } from "./config";

export type DeviceView = {
  id: string; workspace_id: string; name: string; hostname: string; platform: string; arch: string;
  agent_version: string; capabilities: string[]; last_seen: number | null; created_at: number;
  workspace_name: string; online: boolean; active_processes: number; role?: "owner" | "operator" | "viewer";
  identity_public_key?: string; transport_public_key?: string;
};

type DeviceRow = Omit<DeviceView, "capabilities" | "online"> & { capabilities: string };

export type AgentEnrollInput = {
  token: string; name?: string; hostname?: string; platform?: string; arch?: string; publicKey: string;
  transportPublicKey?: string; agentVersion?: string; capabilities?: string[];
};

export function nodeUpdateAvailable(agent: string, rc: string) {
  const parse = (value: string) => value.match(/^(\d+)\.(\d+)\.(\d+)/)?.slice(1).map(Number);
  const current = parse(agent), target = parse(rc);
  if (!current || !target) return agent !== rc;
  for (let index = 0; index < 3; index++) {
    if (current[index] !== target[index]) return current[index] < target[index];
  }
  return false;
}

export function enrollAgent(input: AgentEnrollInput) {
  const token = input.token.trim();
  const enrollment = q<any>(`SELECT * FROM enrollment_tokens
    WHERE token_hash=? AND used_at IS NULL AND expires_at>?`).get(sha(token), now());
  if (!enrollment) throw new HttpError(401, "invalid or expired enrollment token");
  const publicKey = input.publicKey;
  if (!publicKey.includes("BEGIN PUBLIC KEY")) throw new HttpError(400, "invalid public key");
  const transportPublicKey = String(input.transportPublicKey || "");
  if (!/^[A-Za-z0-9_-]{43}$/.test(transportPublicKey)) throw new HttpError(400, "invalid transport public key");
  if (q("SELECT id FROM devices WHERE public_key=?").get(publicKey)) throw new HttpError(409, "device key already enrolled");
  const deviceId = id(), t = now();
  db.transaction(() => {
    const consumed = q("UPDATE enrollment_tokens SET used_at=? WHERE id=? AND used_at IS NULL AND expires_at>?")
      .run(t, enrollment.id, t).changes;
    if (consumed !== 1) throw new HttpError(401, "invalid or expired enrollment token");
    const deviceCount = q<{ count: number }>("SELECT count(*) count FROM devices WHERE workspace_id=?").get(enrollment.workspace_id)?.count || 0;
    if (deviceCount >= MAX_DEVICES_PER_WORKSPACE) throw new HttpError(409, `device limit reached (${MAX_DEVICES_PER_WORKSPACE})`);
    q(`INSERT INTO devices(id,workspace_id,name,hostname,platform,arch,public_key,transport_public_key,agent_version,capabilities,last_seen,created_at)
      VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`).run(
      deviceId, enrollment.workspace_id, String(input.name || input.hostname || "Device").slice(0, 120),
      String(input.hostname || "unknown").slice(0, 255), String(input.platform || "unknown").slice(0, 40),
      String(input.arch || "unknown").slice(0, 40), publicKey, transportPublicKey, String(input.agentVersion || "unknown").slice(0, 40),
      JSON.stringify(input.capabilities || []), t, t,
    );
  })();
  logEvent("device.enrolled", enrollment.workspace_id, enrollment.created_by, deviceId);
  return { deviceId };
}

export async function handleAgentUnregister(req: Request, url: URL): Promise<Response | null> {
  if (url.pathname !== "/api/v1/agent/self" || !["GET", "DELETE"].includes(req.method)) return null;
  const requestedDevice = url.searchParams.get("device") || "";
  if (!requestedDevice) return fail("device not found", 404);
  const active = Boolean(q("SELECT 1 FROM devices WHERE id=?").get(requestedDevice));
  const revoked = Boolean(q("SELECT 1 FROM revoked_devices WHERE id=?").get(requestedDevice));
  if (!active && !revoked) return fail("device not found", 404);
  const deviceId = await verifyAgent(req, requestedDevice);
  if (!deviceId) return fail("invalid agent signature", 401);
  if (revoked) return fail("device removed", 410);
  const device = q<any>("SELECT workspace_id,name,agent_version FROM devices WHERE id=?").get(deviceId);
  if (!device) return fail("device not found", 404);
  if (req.method === "GET") return json({ name: device.name, online: isOnline(deviceId), agentVersion: device.agent_version });
  disconnectDevice(deviceId);
  const publicKey = q<{ public_key: string }>("SELECT public_key FROM devices WHERE id=?").get(deviceId)?.public_key || "";
  db.transaction(() => {
    q("INSERT OR REPLACE INTO revoked_devices(id,public_key,revoked_at) VALUES(?,?,?)").run(deviceId, publicKey, now());
    q("DELETE FROM devices WHERE id=?").run(deviceId);
  })();
  logEvent("device.unenrolled", device.workspace_id, null, null, { deviceId });
  return json({ ok: true });
}

export function listDevices(user: User): DeviceView[] {
  return q<DeviceRow>(`SELECT d.id,d.workspace_id,d.name,d.hostname,d.platform,d.arch,d.agent_version,d.capabilities,
    d.last_seen,d.created_at,w.name workspace_name,(SELECT count(*) FROM processes p WHERE p.device_id=d.id AND p.status IN ('starting','running')) active_processes
    FROM devices d JOIN workspaces w ON w.id=d.workspace_id
    JOIN workspace_members wm ON wm.workspace_id=d.workspace_id WHERE wm.user_id=? ORDER BY d.name`).all(user.id)
    .map(row => ({ ...row, online: isOnline(row.id), capabilities: JSON.parse(row.capabilities || "[]") as string[] }));
}

export function getDevice(user: User, deviceId: string): DeviceView | null {
  const device = q<DeviceRow & { public_key: string; transport_public_key: string }>(`SELECT d.id,d.workspace_id,d.name,d.hostname,d.platform,d.arch,d.agent_version,d.capabilities,d.public_key,d.transport_public_key,
    d.last_seen,d.created_at,w.name workspace_name,wm.role,(SELECT count(*) FROM processes p WHERE p.device_id=d.id AND p.status IN ('starting','running')) active_processes
    FROM devices d JOIN workspaces w ON w.id=d.workspace_id
    JOIN workspace_members wm ON wm.workspace_id=d.workspace_id WHERE d.id=? AND wm.user_id=?`).get(deviceId, user.id);
  return device ? { ...device, identity_public_key: device.public_key, transport_public_key: device.transport_public_key,
    online: isOnline(deviceId), capabilities: JSON.parse(device.capabilities || "[]") as string[] } : null;
}

export function removeDevice(user: User, deviceId: string) {
  const role = deviceRole(user.id, deviceId);
  if (!role) throw new HttpError(404, "device not found");
  if (role !== "owner") throw new HttpError(403, "owner required");
  const device = q<{ workspace_id: string; name: string; public_key: string }>("SELECT workspace_id,name,public_key FROM devices WHERE id=?").get(deviceId);
  if (!device) throw new HttpError(404, "device not found");
  db.transaction(() => {
    q("INSERT OR REPLACE INTO revoked_devices(id,public_key,revoked_at) VALUES(?,?,?)").run(deviceId, device.public_key, now());
    q("DELETE FROM devices WHERE id=?").run(deviceId);
  })();
  disconnectDevice(deviceId, true);
  logEvent("device.removed", device.workspace_id, user.id, null, { deviceId, name: device.name });
}

export function renameDevice(user: User, deviceId: string, value: unknown) {
  const role = deviceRole(user.id, deviceId);
  if (!role) throw new HttpError(404, "device not found");
  if (role !== "owner") throw new HttpError(403, "owner required");
  const name = String(value || "").trim().slice(0, 120);
  if (!name) throw new HttpError(400, "device name required");
  const row = q<{ workspace_id: string }>("SELECT workspace_id FROM devices WHERE id=?").get(deviceId);
  if (!row) throw new HttpError(404, "device not found");
  q("UPDATE devices SET name=? WHERE id=?").run(name, deviceId);
  logEvent("device.renamed", row.workspace_id, user.id, deviceId, { name });
  return { name };
}
