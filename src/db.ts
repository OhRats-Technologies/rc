import { Database } from "bun:sqlite";
import { $ } from "bun";
import { DATA_DIR, DB_PATH } from "./config";
import { bytesToBase64url } from "./encoding";

await $`mkdir -p ${DATA_DIR}`.quiet();
export const db = new Database(DB_PATH, { create: true, strict: true });
db.exec("PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000;");

db.exec(`
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
  role TEXT NOT NULL CHECK(role IN ('owner','operator','viewer')),joined_at INTEGER NOT NULL,
  PRIMARY KEY(workspace_id,user_id)
);
CREATE TABLE IF NOT EXISTS devices(
  id TEXT PRIMARY KEY,workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  name TEXT NOT NULL,hostname TEXT NOT NULL,platform TEXT NOT NULL,arch TEXT NOT NULL,
  public_key TEXT NOT NULL UNIQUE,transport_public_key TEXT NOT NULL DEFAULT '',lock_hash TEXT NOT NULL DEFAULT '',agent_version TEXT NOT NULL,capabilities TEXT NOT NULL DEFAULT '[]',
  lock_generation INTEGER NOT NULL DEFAULT 0,last_seen INTEGER,created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS processes(
  id TEXT PRIMARY KEY,device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
  command TEXT NOT NULL,cwd TEXT,status TEXT NOT NULL DEFAULT 'starting'
    CHECK(status IN ('starting','running','exited','lost')),
  encrypted INTEGER NOT NULL DEFAULT 0,
  output_head TEXT NOT NULL DEFAULT '',output_tail TEXT NOT NULL DEFAULT '',output_chars INTEGER NOT NULL DEFAULT 0,
  revision INTEGER NOT NULL DEFAULT 0,cols INTEGER NOT NULL DEFAULT 80,rows INTEGER NOT NULL DEFAULT 24,
  exit_code INTEGER,signal TEXT,error TEXT,created_by TEXT NOT NULL REFERENCES users(id),
  created_at INTEGER NOT NULL,started_at INTEGER,completed_at INTEGER
);
CREATE TABLE IF NOT EXISTS auth_sessions(
  token_hash TEXT PRIMARY KEY,user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at INTEGER NOT NULL,expires_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS step_up_authorizations(
  id TEXT PRIMARY KEY,user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  challenge TEXT NOT NULL UNIQUE,created_at INTEGER NOT NULL,expires_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS step_up_tokens(
  token_hash TEXT PRIMARY KEY,user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at INTEGER NOT NULL,expires_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS api_tokens(
  id TEXT PRIMARY KEY,user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name TEXT NOT NULL,token_hash TEXT NOT NULL UNIQUE,public_key TEXT NOT NULL DEFAULT '',scopes TEXT NOT NULL DEFAULT '["read","execute"]',
  created_at INTEGER NOT NULL,last_used INTEGER
);
CREATE TABLE IF NOT EXISTS api_request_nonces(
  token_id TEXT NOT NULL REFERENCES api_tokens(id) ON DELETE CASCADE,nonce_hash TEXT NOT NULL,
  expires_at INTEGER NOT NULL,PRIMARY KEY(token_id,nonce_hash)
);
CREATE TABLE IF NOT EXISTS control_authorizations(
  id TEXT PRIMARY KEY,user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  client_id TEXT NOT NULL,signing_public_key TEXT NOT NULL,grant TEXT NOT NULL,challenge TEXT NOT NULL,
  created_at INTEGER NOT NULL,expires_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS control_clients(
  id TEXT PRIMARY KEY,user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  signing_public_key TEXT NOT NULL,credential_id TEXT NOT NULL,grant TEXT NOT NULL,assertion TEXT NOT NULL,
  created_at INTEGER NOT NULL,expires_at INTEGER NOT NULL,last_used INTEGER
);
CREATE TABLE IF NOT EXISTS cli_authorizations(
  id TEXT PRIMARY KEY,device_code_hash TEXT NOT NULL UNIQUE,user_code_hash TEXT NOT NULL UNIQUE,
  client_id TEXT NOT NULL DEFAULT '',signing_public_key TEXT NOT NULL DEFAULT '',
  user_id TEXT REFERENCES users(id) ON DELETE CASCADE,created_at INTEGER NOT NULL,expires_at INTEGER NOT NULL,
  approved_at INTEGER,exchanged_at INTEGER
);
CREATE TABLE IF NOT EXISTS cli_sessions(
  token_hash TEXT PRIMARY KEY,user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at INTEGER NOT NULL,expires_at INTEGER NOT NULL,last_used INTEGER
);
CREATE TABLE IF NOT EXISTS workspace_invites(
  id TEXT PRIMARY KEY,workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  token_hash TEXT NOT NULL UNIQUE,role TEXT NOT NULL CHECK(role IN ('operator','viewer')),
  created_by TEXT NOT NULL REFERENCES users(id),created_at INTEGER NOT NULL,expires_at INTEGER NOT NULL,used_at INTEGER
);
CREATE TABLE IF NOT EXISTS enrollment_tokens(
  id TEXT PRIMARY KEY,workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  token_hash TEXT NOT NULL UNIQUE,created_by TEXT NOT NULL REFERENCES users(id),
  created_at INTEGER NOT NULL,expires_at INTEGER NOT NULL,used_at INTEGER
);
CREATE TABLE IF NOT EXISTS agent_auth_challenges(
  challenge_hash TEXT PRIMARY KEY,device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
  created_at INTEGER NOT NULL,expires_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS events(
  id INTEGER PRIMARY KEY AUTOINCREMENT,workspace_id TEXT REFERENCES workspaces(id) ON DELETE CASCADE,
  user_id TEXT REFERENCES users(id) ON DELETE SET NULL,device_id TEXT REFERENCES devices(id) ON DELETE SET NULL,
  kind TEXT NOT NULL,detail TEXT NOT NULL DEFAULT '{}',created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS actions(
  id TEXT PRIMARY KEY,workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  name TEXT NOT NULL,description TEXT NOT NULL DEFAULT '',command TEXT NOT NULL,cwd TEXT,
  confirm INTEGER NOT NULL DEFAULT 0,created_by TEXT NOT NULL REFERENCES users(id),
  created_at INTEGER NOT NULL,updated_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_members_user ON workspace_members(user_id);
CREATE INDEX IF NOT EXISTS idx_passkeys_user ON passkeys(user_id);
CREATE INDEX IF NOT EXISTS idx_devices_workspace ON devices(workspace_id);
CREATE INDEX IF NOT EXISTS idx_processes_device ON processes(device_id,created_at);
CREATE INDEX IF NOT EXISTS idx_processes_status ON processes(device_id,status);
CREATE INDEX IF NOT EXISTS idx_events_workspace ON events(workspace_id,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_actions_workspace ON actions(workspace_id,name);
CREATE INDEX IF NOT EXISTS idx_cli_authorizations_expiry ON cli_authorizations(expires_at);
CREATE INDEX IF NOT EXISTS idx_cli_sessions_user ON cli_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_agent_auth_challenges_expiry ON agent_auth_challenges(expires_at);
CREATE INDEX IF NOT EXISTS idx_api_request_nonces_expiry ON api_request_nonces(expires_at);
CREATE INDEX IF NOT EXISTS idx_control_authorizations_expiry ON control_authorizations(expires_at);
CREATE INDEX IF NOT EXISTS idx_control_clients_user ON control_clients(user_id,expires_at);
`);

function migrateRoleVocabulary() {
  const memberSql = db.query<{ sql: string }, []>("SELECT sql FROM sqlite_master WHERE type='table' AND name='workspace_members'").get()?.sql || "";
  if (memberSql.includes("'member'")) db.transaction(() => {
    db.exec(`CREATE TABLE workspace_members_v2(
      workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
      user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
      role TEXT NOT NULL CHECK(role IN ('owner','operator','viewer')),joined_at INTEGER NOT NULL,
      PRIMARY KEY(workspace_id,user_id));`);
    db.exec("INSERT INTO workspace_members_v2 SELECT workspace_id,user_id,CASE role WHEN 'member' THEN 'operator' ELSE role END,joined_at FROM workspace_members;");
    db.exec("DROP TABLE workspace_members; ALTER TABLE workspace_members_v2 RENAME TO workspace_members;");
  })();
  const inviteSql = db.query<{ sql: string }, []>("SELECT sql FROM sqlite_master WHERE type='table' AND name='workspace_invites'").get()?.sql || "";
  if (inviteSql.includes("'member'")) db.transaction(() => {
    db.exec(`CREATE TABLE workspace_invites_v2(
      id TEXT PRIMARY KEY,workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
      token_hash TEXT NOT NULL UNIQUE,role TEXT NOT NULL CHECK(role IN ('operator','viewer')),
      created_by TEXT NOT NULL REFERENCES users(id),created_at INTEGER NOT NULL,expires_at INTEGER NOT NULL,used_at INTEGER);`);
    db.exec("INSERT INTO workspace_invites_v2 SELECT id,workspace_id,token_hash,CASE role WHEN 'member' THEN 'operator' ELSE role END,created_by,created_at,expires_at,used_at FROM workspace_invites;");
    db.exec("DROP TABLE workspace_invites; ALTER TABLE workspace_invites_v2 RENAME TO workspace_invites;");
  })();
  db.exec("CREATE INDEX IF NOT EXISTS idx_members_user ON workspace_members(user_id);");
}

migrateRoleVocabulary();

function migrateApiTokenScopes() {
  const columns = db.query<{ name: string }, []>("PRAGMA table_info(api_tokens)").all().map(row => row.name);
  if (!columns.includes("scopes")) db.exec(`ALTER TABLE api_tokens ADD COLUMN scopes TEXT NOT NULL DEFAULT '["read","execute"]'`);
  if (!columns.includes("public_key")) db.exec(`ALTER TABLE api_tokens ADD COLUMN public_key TEXT NOT NULL DEFAULT ''`);
  db.exec("DELETE FROM api_tokens WHERE public_key=''");
}

migrateApiTokenScopes();

function migrateDeviceTransportKeys() {
  const columns = db.query<{ name: string }, []>("PRAGMA table_info(devices)").all().map(row => row.name);
  if (!columns.includes("transport_public_key")) db.exec("ALTER TABLE devices ADD COLUMN transport_public_key TEXT NOT NULL DEFAULT ''");
  if (!columns.includes("lock_hash")) db.exec("ALTER TABLE devices ADD COLUMN lock_hash TEXT NOT NULL DEFAULT ''");
  if (!columns.includes("lock_generation")) db.exec("ALTER TABLE devices ADD COLUMN lock_generation INTEGER NOT NULL DEFAULT 0");
}

migrateDeviceTransportKeys();

function migrateEncryptedProcesses() {
  const columns = db.query<{ name: string }, []>("PRAGMA table_info(processes)").all().map(row => row.name);
  if (!columns.includes("encrypted")) db.exec("ALTER TABLE processes ADD COLUMN encrypted INTEGER NOT NULL DEFAULT 0");
}

migrateEncryptedProcesses();

function migrateCliControlKeys() {
  const columns = db.query<{ name: string }, []>("PRAGMA table_info(cli_authorizations)").all().map(row => row.name);
  if (!columns.includes("client_id")) db.exec("ALTER TABLE cli_authorizations ADD COLUMN client_id TEXT NOT NULL DEFAULT ''");
  if (!columns.includes("signing_public_key")) db.exec("ALTER TABLE cli_authorizations ADD COLUMN signing_public_key TEXT NOT NULL DEFAULT ''");
}

migrateCliControlKeys();

export const q = <T = any>(sql: string) => db.query<T, any[]>(sql);
export const now = () => Date.now();
export const id = () => crypto.randomUUID();
export const sha = (value: string) => new Bun.CryptoHasher("sha256").update(value).digest("hex");
export const opaque = (prefix: string) => `${prefix}_${bytesToBase64url(crypto.getRandomValues(new Uint8Array(32)))}`;
