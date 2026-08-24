import { User, canWrite, devicePermission, deviceRole, logEvent, sessionAccess } from "./core";
import { db, id, now, q, sha } from "./db";
import { disconnectDevice, dispatchJob, isOnline, verifyAgent } from "./gateway";
import { publishEvent } from "./events";
import { body, fail, json } from "./http-utils";

export async function handleAgentEnroll(req: Request, path: string): Promise<Response | null> {
  if (path !== "/api/v1/agent/enroll" || req.method !== "POST") return null;
  const input = await body(req), token = String(input.token || "").trim();
  const enrollment = q<any>(`SELECT et.*,f.workspace_id FROM enrollment_tokens et JOIN fleets f ON f.id=et.fleet_id
    WHERE et.token_hash=? AND et.used_at IS NULL AND et.expires_at>?`).get(sha(token), now());
  if (!enrollment) return fail("invalid or expired enrollment token", 401);
  const publicKey = String(input.publicKey || "");
  if (!publicKey.includes("BEGIN PUBLIC KEY")) return fail("invalid public key");
  if (q("SELECT id FROM devices WHERE public_key=?").get(publicKey)) return fail("device key already enrolled", 409);
  const deviceId = id(), t = now();
  db.transaction(() => {
    q(`INSERT INTO devices(id,name,hostname,platform,arch,public_key,agent_version,capabilities,last_seen,created_at)
      VALUES(?,?,?,?,?,?,?,?,?,?)`).run(
      deviceId, String(input.name || input.hostname || "Device").slice(0, 120),
      String(input.hostname || "unknown").slice(0, 255), String(input.platform || "unknown").slice(0, 40),
      String(input.arch || "unknown").slice(0, 40), publicKey, String(input.agentVersion || "unknown").slice(0, 40),
      JSON.stringify(Array.isArray(input.capabilities) ? input.capabilities : []), t, t
    );
    q("INSERT INTO fleet_devices(fleet_id,device_id,permissions,joined_at) VALUES(?,?,?,?)")
      .run(enrollment.fleet_id, deviceId, JSON.stringify(["shell"]), t);
    q("UPDATE enrollment_tokens SET used_at=? WHERE id=?").run(t, enrollment.id);
  })();
  logEvent("device.enrolled", enrollment.workspace_id, enrollment.created_by, deviceId, { fleetId: enrollment.fleet_id });
  return json({ deviceId }, 201);
}

export async function handleAgentUnregister(req: Request, url: URL): Promise<Response | null> {
  if (url.pathname !== "/api/v1/agent/self" || req.method !== "DELETE") return null;
  const deviceId = verifyAgent(url);
  if (!deviceId) return fail("invalid agent signature", 401);
  const workspace = q<any>(`SELECT f.workspace_id FROM fleet_devices fd JOIN fleets f ON f.id=fd.fleet_id
    WHERE fd.device_id=? LIMIT 1`).get(deviceId)?.workspace_id || null;
  disconnectDevice(deviceId);
  q("DELETE FROM devices WHERE id=?").run(deviceId);
  logEvent("device.unenrolled", workspace, null, null, { deviceId });
  return json({ ok: true });
}

function listDevices(user: User) {
  return q<any>(`SELECT d.id,d.name,d.hostname,d.platform,d.arch,d.agent_version,d.capabilities,d.last_seen,d.created_at,
    group_concat(DISTINCT w.name) workspaces,group_concat(DISTINCT f.name) fleets FROM devices d
    JOIN fleet_devices fd ON fd.device_id=d.id JOIN fleets f ON f.id=fd.fleet_id
    JOIN workspaces w ON w.id=f.workspace_id JOIN workspace_members wm ON wm.workspace_id=w.id
    WHERE wm.user_id=? GROUP BY d.id ORDER BY d.name`).all(user.id)
    .map((row: any) => ({ ...row, online: isOnline(row.id), capabilities: JSON.parse(row.capabilities || "[]") }));
}

function deviceDetail(user: User, deviceId: string) {
  if (!deviceRole(user.id, deviceId)) return null;
  const device = q<any>(`SELECT id,name,hostname,platform,arch,agent_version,capabilities,last_seen,created_at FROM devices WHERE id=?`).get(deviceId);
  if (!device) return null;
  const fleets = q<any>(`SELECT f.id,f.name,w.id workspace_id,w.name workspace_name FROM fleets f
    JOIN fleet_devices fd ON fd.fleet_id=f.id JOIN workspaces w ON w.id=f.workspace_id
    JOIN workspace_members wm ON wm.workspace_id=w.id WHERE fd.device_id=? AND wm.user_id=? ORDER BY w.name,f.name`).all(deviceId, user.id);
  return { ...device, online: isOnline(deviceId), capabilities: JSON.parse(device.capabilities || "[]"), fleets };
}

async function createJob(req: Request, user: User, sessionId: string) {
  const access = sessionAccess(user.id, sessionId);
  if (!access || !canWrite(access.role)) return fail("forbidden", 403);
  const input = await body(req), command = String(input.command || "").trim();
  if (!command) return fail("command required");
  if (command.length > 8192) return fail("command too long");
  const jobId = id(), t = now();
  q("INSERT INTO jobs(id,device_id,session_id,type,payload,status,created_by,created_at) VALUES(?,?,?,?,?,?,?,?)")
    .run(jobId, access.deviceId, sessionId, "shell", JSON.stringify({ command }), "pending", user.id, t);
  dispatchJob(jobId, access.deviceId, command);
  const workspace = q<any>(`SELECT f.workspace_id FROM fleet_devices fd JOIN fleets f ON f.id=fd.fleet_id
    WHERE fd.device_id=? LIMIT 1`).get(access.deviceId);
  logEvent("job.created", workspace?.workspace_id || null, user.id, access.deviceId, { jobId, command });
  publishEvent({ kind: "job.updated", workspaceId: workspace?.workspace_id || null, deviceId: access.deviceId, sessionId, jobId });
  return json({ id: jobId }, 201);
}

export async function handleDevices(req: Request, path: string, user: User): Promise<Response | null> {
  if (path === "/api/v1/devices" && req.method === "GET") return json({ devices: listDevices(user) });
  let m = path.match(/^\/api\/v1\/devices\/([^/]+)$/);
  if (m && req.method === "GET") { const data = deviceDetail(user, m[1]); return data ? json({ device: data }) : fail("device not found", 404); }
  m = path.match(/^\/api\/v1\/devices\/([^/]+)\/sessions$/);
  if (m && req.method === "POST") {
    const role = deviceRole(user.id, m[1]);
    if (!canWrite(role) || !devicePermission(user.id, m[1], "shell")) return fail("forbidden", 403);
    const sessionId = id();
    q("INSERT INTO sessions(id,device_id,user_id,type,status,created_at) VALUES(?,?,?,?,?,?)")
      .run(sessionId, m[1], user.id, "shell", "active", now());
    return json({ id: sessionId }, 201);
  }
  m = path.match(/^\/api\/v1\/sessions\/([^/]+)\/jobs$/);
  if (m && req.method === "POST") return createJob(req, user, m[1]);
  if (m && req.method === "GET") {
    const access = sessionAccess(user.id, m[1]); if (!access?.role) return fail("forbidden", 403);
    const jobs = q<any>(`SELECT id,type,payload,status,result,exit_code,created_at,started_at,completed_at FROM jobs
      WHERE session_id=? ORDER BY created_at`).all(m[1]).map((job: any) => ({ ...job, payload: JSON.parse(job.payload) }));
    return json({ jobs });
  }
  m = path.match(/^\/api\/v1\/sessions\/([^/]+)$/);
  if (m && req.method === "DELETE") {
    const access = sessionAccess(user.id, m[1]); if (!access || !canWrite(access.role)) return fail("forbidden", 403);
    q("UPDATE sessions SET status='closed',closed_at=? WHERE id=?").run(now(), m[1]);
    return json({ ok: true });
  }
  return null;
}
