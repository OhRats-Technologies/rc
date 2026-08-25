import type { User } from "./core";
import { db, now, q } from "./db";
import { HttpError } from "./errors";
import { deleteWorkspace } from "./workspaces";

export const DELETED_USER_ID = "__deleted_account__";

export function activeUserCount() {
  return q<{ count: number }>("SELECT count(*) count FROM users WHERE id<>?").get(DELETED_USER_ID)?.count || 0;
}

export function renameUser(user: User, value: unknown) {
  const name = String(value || "").trim().slice(0, 120);
  if (!name) throw new HttpError(400, "account name required");
  if (!q("UPDATE users SET name=? WHERE id=? AND id<>?").run(name, user.id, DELETED_USER_ID).changes) {
    throw new HttpError(404, "account not found");
  }
  return { name };
}

export function deleteUser(user: User) {
  const blockers = q<{ name: string }>(`SELECT w.name FROM workspaces w
    JOIN workspace_members me ON me.workspace_id=w.id AND me.user_id=? AND me.role='owner'
    WHERE (SELECT count(*) FROM workspace_members owners WHERE owners.workspace_id=w.id AND owners.role='owner')=1
      AND (SELECT count(*) FROM workspace_members members WHERE members.workspace_id=w.id)>1
    ORDER BY w.name`).all(user.id);
  if (blockers.length) {
    throw new HttpError(409, `Promote another owner before deleting your account: ${blockers.map(row => row.name).join(", ")}`);
  }

  const privateWorkspaces = q<{ id: string }>(`SELECT w.id FROM workspaces w
    JOIN workspace_members me ON me.workspace_id=w.id AND me.user_id=? AND me.role='owner'
    WHERE (SELECT count(*) FROM workspace_members members WHERE members.workspace_id=w.id)=1`).all(user.id);
  for (const workspace of privateWorkspaces) deleteWorkspace(user, workspace.id);

  db.transaction(() => {
    q("INSERT OR IGNORE INTO users(id,name,created_at) VALUES(?,?,?)").run(DELETED_USER_ID, "Deleted account", now());
    for (const table of ["workspaces", "processes", "workspace_invites", "enrollment_tokens", "actions"]) {
      q(`UPDATE ${table} SET created_by=? WHERE created_by=?`).run(DELETED_USER_ID, user.id);
    }
    q("DELETE FROM users WHERE id=? AND id<>?").run(user.id, DELETED_USER_ID);
  })();
}
