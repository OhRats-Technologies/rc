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
  const ownedWorkspaces = q<{ id: string }>(`SELECT workspace_id id FROM workspace_members
    WHERE user_id=? AND role='owner'`).all(user.id);
  for (const workspace of ownedWorkspaces) deleteWorkspace(user, workspace.id);

  db.transaction(() => {
    q("INSERT OR IGNORE INTO users(id,name,created_at) VALUES(?,?,?)").run(DELETED_USER_ID, "Deleted account", now());
    for (const table of ["workspaces", "processes", "workspace_invites", "enrollment_tokens"]) {
      q(`UPDATE ${table} SET created_by=? WHERE created_by=?`).run(DELETED_USER_ID, user.id);
    }
    q("DELETE FROM users WHERE id=? AND id<>?").run(user.id, DELETED_USER_ID);
  })();
}
