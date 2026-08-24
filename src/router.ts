import { handleTokens } from "./account";
import { auth, handleAccount, handlePublicAuth } from "./auth";
import { User } from "./core";
import { handleAgentEnroll, handleAgentUnregister, handleDevices } from "./devices";
import { eventStream } from "./events";
import { agentsCount } from "./gateway";
import { checkOrigin, fail, json } from "./http-utils";
import { handleWorkspaces } from "./workspaces";

async function authenticated(req: Request, path: string, user: User) {
  if (path === "/api/v1/events" && req.method === "GET") return eventStream(user.id);
  return await handleAccount(req, path, user)
    || await handleTokens(req, path, user)
    || await handleWorkspaces(req, path, user)
    || await handleDevices(req, path, user)
    || fail("not found", 404);
}

export async function handleAPI(req: Request, url: URL): Promise<Response> {
  if (!checkOrigin(req)) return fail("invalid origin", 403);
  const path = url.pathname;
  if (path === "/api/v1/health" && req.method === "GET") return json({ ok: true, version: "0.1.0", agents: agentsCount() });
  const publicAuth = await handlePublicAuth(req, path);
  if (publicAuth) return publicAuth;
  const enroll = await handleAgentEnroll(req, path);
  if (enroll) return enroll;
  const unregister = await handleAgentUnregister(req, url);
  if (unregister) return unregister;
  const user = await auth(req);
  if (!user) return fail("authentication required", 401);
  return authenticated(req, path, user);
}
