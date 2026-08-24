import { cookieUser } from "./auth";
import { cookie } from "./http-utils";
import { userWorkspaces, type User } from "./core";
import type { WorkspaceView } from "./workspaces";

export type PageContext = { user: User; workspaces: WorkspaceView[]; sidebar: "open" | "closed" };

export async function pageContext(request: Request): Promise<PageContext | null> {
  const user = await cookieUser(request);
  if (!user) return null;
  const sidebar = cookie(request, "relay_sidebar") === "closed" ? "closed" : "open";
  return { user, workspaces: userWorkspaces(user.id) as WorkspaceView[], sidebar };
}

export function safeNext(value: unknown) {
  const next = String(value || "/devices");
  return next.startsWith("/") && !next.startsWith("//") ? next : "/devices";
}
