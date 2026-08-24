import { logEvent, roleFor, type Role, type User } from "./core";
import { now, q } from "./db";
import { HttpError } from "./errors";

export type MemberView = { user_id: string; name: string; role: Role; joined_at: number };
export type InviteView = { id: string; role: "operator" | "viewer"; created_at: number; expires_at: number };

function requireOwner(user: User, workspaceId: string) {
  if (roleFor(user.id, workspaceId) !== "owner") throw new HttpError(403, "owner required");
}

function ownerCount(workspaceId: string) {
  return q<{ count: number }>("SELECT count(*) count FROM workspace_members WHERE workspace_id=? AND role='owner'").get(workspaceId)?.count || 0;
}

export function workspaceAccess(user: User, workspaceId: string) {
  requireOwner(user, workspaceId);
  const members = q<MemberView>(`SELECT wm.user_id,u.name,wm.role,wm.joined_at FROM workspace_members wm
    JOIN users u ON u.id=wm.user_id WHERE wm.workspace_id=? ORDER BY wm.joined_at`).all(workspaceId);
  const invites = q<InviteView>(`SELECT id,role,created_at,expires_at FROM workspace_invites
    WHERE workspace_id=? AND used_at IS NULL AND expires_at>? ORDER BY created_at DESC`).all(workspaceId, now());
  return { members, invites };
}

export function changeWorkspaceRole(user: User, workspaceId: string, memberId: string, next: unknown) {
  requireOwner(user, workspaceId);
  const role = next === "owner" || next === "viewer" ? next : "operator";
  const current = q<{ role: Role }>("SELECT role FROM workspace_members WHERE workspace_id=? AND user_id=?").get(workspaceId, memberId);
  if (!current) throw new HttpError(404, "member not found");
  if (current.role === "owner" && role !== "owner" && ownerCount(workspaceId) <= 1) throw new HttpError(409, "promote another owner first");
  q("UPDATE workspace_members SET role=? WHERE workspace_id=? AND user_id=?").run(role, workspaceId, memberId);
  logEvent("workspace.member.role.changed", workspaceId, user.id, null, { memberId, role });
  return { role };
}

export function removeWorkspaceMember(user: User, workspaceId: string, memberId: string) {
  requireOwner(user, workspaceId);
  if (memberId === user.id) throw new HttpError(409, "use Leave workspace for your own account");
  const member = q<{ role: Role; name: string }>(`SELECT wm.role,u.name FROM workspace_members wm JOIN users u ON u.id=wm.user_id
    WHERE wm.workspace_id=? AND wm.user_id=?`).get(workspaceId, memberId);
  if (!member) throw new HttpError(404, "member not found");
  if (member.role === "owner" && ownerCount(workspaceId) <= 1) throw new HttpError(409, "workspace needs an owner");
  q("DELETE FROM workspace_members WHERE workspace_id=? AND user_id=?").run(workspaceId, memberId);
  logEvent("workspace.member.removed", workspaceId, user.id, null, { memberId, name: member.name });
}

export function leaveWorkspace(user: User, workspaceId: string) {
  const role = roleFor(user.id, workspaceId);
  if (!role) throw new HttpError(404, "workspace not found");
  if (role === "owner" && ownerCount(workspaceId) <= 1) throw new HttpError(409, "promote another owner before leaving");
  q("DELETE FROM workspace_members WHERE workspace_id=? AND user_id=?").run(workspaceId, user.id);
  logEvent("workspace.member.left", workspaceId, user.id, null, { role });
}

export function revokeInvite(user: User, workspaceId: string, inviteId: string) {
  requireOwner(user, workspaceId);
  if (!q("DELETE FROM workspace_invites WHERE id=? AND workspace_id=? AND used_at IS NULL").run(inviteId, workspaceId).changes) {
    throw new HttpError(404, "invite not found");
  }
  logEvent("workspace.invite.revoked", workspaceId, user.id, null, { inviteId });
}
