export type Role = "owner" | "operator" | "viewer";
export type ProcessStatus = "starting" | "running" | "exited" | "lost";

export interface User { id: string; name: string }
export interface Workspace { id: string; name: string; role: Role; created_at: number }
export interface Me { user: User; workspaces: Workspace[] }
export interface Status { setupRequired: boolean; setupAuthorized: boolean; version: string }

export interface Device {
  id: string;
  workspace_id: string;
  workspace_name: string;
  name: string;
  hostname: string;
  platform: string;
  arch: string;
  agent_version: string;
  capabilities: string[];
  last_seen: number | null;
  created_at: number;
  online: boolean;
  active_processes: number;
  role?: Role;
  identity_public_key?: string;
  transport_public_key?: string;
}

export interface RemoteProcess {
  id: string;
  device_id: string;
  command: string;
  encrypted?: boolean;
  terminal?: boolean;
  cwd: string | null;
  status: ProcessStatus;
  output: string;
  revision: number;
  cols: number;
  rows: number;
  exit_code: number | null;
  signal: string | null;
  error: string | null;
  created_by: string;
  created_by_name: string | null;
  created_at: number;
  started_at: number | null;
  completed_at: number | null;
}

export interface RCEvent {
  kind: string;
  workspaceId?: string | null;
  deviceId?: string | null;
  processId?: string | null;
  audit?: boolean;
  detail?: Record<string, unknown>;
  at?: number;
  created_at?: number;
  device_id?: string | null;
}

export interface Passkey { id: string; created_at: number; last_used: number | null }
export interface ApiToken { id: string; name: string; created_at: number; last_used: number | null }
