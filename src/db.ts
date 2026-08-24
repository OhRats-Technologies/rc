import { Database } from "bun:sqlite";
import { createHash, randomBytes } from "node:crypto";
import { mkdirSync } from "node:fs";
import { DATA_DIR, DB_PATH } from "./config";

mkdirSync(DATA_DIR, { recursive: true });
export const db = new Database(DB_PATH, { create: true, strict: true });
db.exec("PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000;");

db.exec(`
CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY,applied_at INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS users(id TEXT PRIMARY KEY,name TEXT NOT NULL,created_at INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS passkeys(
  id TEXT PRIMARY KEY,user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  credential_id TEXT NOT NULL UNIQUE,public_key TEXT NOT NULL,counter INTEGER NOT NULL DEFAULT 0,
  transports TEXT NOT NULL DEFAULT '[]',created_at INTEGER NOT NULL,last_used INTEGER
);
CREATE TABLE IF NOT EXISTS webauthn_challenges(
  id TEXT PRIMARY KEY,challenge TEXT NOT NULL UNIQUE,
  kind TEXT NOT NULL CHECK(kind IN ('setup','register','login','add-passkey')),
  user_id TEXT,name TEXT,invite_id TEXT,created_at INTEGER NOT NULL,expires_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS workspaces(
  id TEXT PRIMARY KEY,name TEXT NOT NULL,created_by TEXT NOT NULL REFERENCES users(id),created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS workspace_members(
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role TEXT NOT NULL CHECK(role IN ('owner','member','viewer')),joined_at INTEGER NOT NULL,
  PRIMARY KEY(workspace_id,user_id)
);
CREATE TABLE IF NOT EXISTS fleets(
  id TEXT PRIMARY KEY,workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  name TEXT NOT NULL,created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS devices(
  id TEXT PRIMARY KEY,name TEXT NOT NULL,hostname TEXT NOT NULL,platform TEXT NOT NULL,arch TEXT NOT NULL,
  public_key TEXT NOT NULL UNIQUE,agent_version TEXT NOT NULL,capabilities TEXT NOT NULL DEFAULT '[]',
  last_seen INTEGER,created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS fleet_devices(
  fleet_id TEXT NOT NULL REFERENCES fleets(id) ON DELETE CASCADE,
  device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
  permissions TEXT NOT NULL DEFAULT '["shell"]',joined_at INTEGER NOT NULL,
  PRIMARY KEY(fleet_id,device_id)
);
CREATE TABLE IF NOT EXISTS sessions(
  id TEXT PRIMARY KEY,device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
  user_id TEXT NOT NULL REFERENCES users(id),type TEXT NOT NULL DEFAULT 'shell',
  status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','closed')),
  created_at INTEGER NOT NULL,closed_at INTEGER
);
CREATE TABLE IF NOT EXISTS jobs(
  id TEXT PRIMARY KEY,device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
  session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,type TEXT NOT NULL DEFAULT 'shell',
  payload TEXT NOT NULL,status TEXT NOT NULL DEFAULT 'pending'
    CHECK(status IN ('pending','sent','completed','failed','cancelled')),
  result TEXT,exit_code INTEGER,created_by TEXT NOT NULL REFERENCES users(id),created_at INTEGER NOT NULL,
  started_at INTEGER,completed_at INTEGER
);
CREATE TABLE IF NOT EXISTS auth_sessions(
  token_hash TEXT PRIMARY KEY,user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at INTEGER NOT NULL,expires_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS api_tokens(
  id TEXT PRIMARY KEY,user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name TEXT NOT NULL,token_hash TEXT NOT NULL UNIQUE,created_at INTEGER NOT NULL,last_used INTEGER
);
CREATE TABLE IF NOT EXISTS workspace_invites(
  id TEXT PRIMARY KEY,workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  token_hash TEXT NOT NULL UNIQUE,role TEXT NOT NULL CHECK(role IN ('member','viewer')),
  created_by TEXT NOT NULL REFERENCES users(id),created_at INTEGER NOT NULL,expires_at INTEGER NOT NULL,used_at INTEGER
);
CREATE TABLE IF NOT EXISTS enrollment_tokens(
  id TEXT PRIMARY KEY,fleet_id TEXT NOT NULL REFERENCES fleets(id) ON DELETE CASCADE,
  token_hash TEXT NOT NULL UNIQUE,created_by TEXT NOT NULL REFERENCES users(id),
  created_at INTEGER NOT NULL,expires_at INTEGER NOT NULL,used_at INTEGER
);
CREATE TABLE IF NOT EXISTS events(
  id INTEGER PRIMARY KEY AUTOINCREMENT,workspace_id TEXT REFERENCES workspaces(id) ON DELETE CASCADE,
  user_id TEXT REFERENCES users(id) ON DELETE SET NULL,device_id TEXT REFERENCES devices(id) ON DELETE SET NULL,
  kind TEXT NOT NULL,detail TEXT NOT NULL DEFAULT '{}',created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_members_user ON workspace_members(user_id);
CREATE INDEX IF NOT EXISTS idx_passkeys_user ON passkeys(user_id);
CREATE INDEX IF NOT EXISTS idx_fleets_workspace ON fleets(workspace_id);
CREATE INDEX IF NOT EXISTS idx_fleet_devices_device ON fleet_devices(device_id);
CREATE INDEX IF NOT EXISTS idx_sessions_device ON sessions(device_id);
CREATE INDEX IF NOT EXISTS idx_jobs_device_status ON jobs(device_id,status);
CREATE INDEX IF NOT EXISTS idx_events_workspace ON events(workspace_id,created_at DESC);
`);

export const q = <T = any>(sql: string) => db.query<T, any[]>(sql);
export const now = () => Date.now();
export const id = () => crypto.randomUUID();
export const sha = (value: string) => createHash("sha256").update(value).digest("hex");
export const opaque = (prefix: string) => `${prefix}_${randomBytes(32).toString("base64url")}`;

if (!q<any>("SELECT version FROM schema_migrations WHERE version=1").get()) {
  q("INSERT INTO schema_migrations(version,applied_at) VALUES(1,?)").run(now());
}
const oldUsers = q<any>("PRAGMA table_info(users)").all();
if (oldUsers.some((column: any) => column.name === "email" || column.name === "password_hash")) {
  db.exec("PRAGMA foreign_keys=OFF");
  try {
    db.transaction(() => db.exec(`
      CREATE TABLE users_v2(id TEXT PRIMARY KEY,name TEXT NOT NULL,created_at INTEGER NOT NULL);
      INSERT INTO users_v2(id,name,created_at) SELECT id,name,created_at FROM users;
      DROP TABLE users; ALTER TABLE users_v2 RENAME TO users;
    `))();
  } finally { db.exec("PRAGMA foreign_keys=ON"); }
}
if (!q<any>("SELECT version FROM schema_migrations WHERE version=2").get()) {
  q("INSERT INTO schema_migrations(version,applied_at) VALUES(2,?)").run(now());
}
