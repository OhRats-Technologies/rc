import shell from "../../web/index.html";
import { Elysia } from "elysia";
import { frontendHTML } from "../artifacts";
import { PUBLIC_URL, SETUP_TOKEN } from "../config";
import { q, sha } from "../db";
import { fail, setupCookie } from "../http-utils";

const production = Bun.env.NODE_ENV === "production";
export const spaRoutes = new Elysia({ name: "relay.pages.spa" });

const prodPage = () => frontendHTML(import.meta.dir);
if (production) {
  spaRoutes.get("/", prodPage).get("/devices", prodPage).get("/devices/*", prodPage)
    .get("/workspaces", prodPage).get("/workspaces/*", prodPage).get("/account", prodPage).get("/api", prodPage);
} else {
  spaRoutes.get("/", shell).get("/devices", shell).get("/devices/*", shell)
    .get("/workspaces", shell).get("/workspaces/*", shell).get("/account", shell).get("/api", shell);
}

spaRoutes.get("/setup/:token", ({ params }) => {
  if ((q<{ count: number }>("SELECT count(*) count FROM users").get()?.count || 0) > 0) return Response.redirect(PUBLIC_URL + "/", 303);
  if (!SETUP_TOKEN || sha(params.token) !== sha(SETUP_TOKEN)) return fail("invalid setup link", 403);
  return new Response(null, {
    status: 303,
    headers: { location: "/", "set-cookie": setupCookie(params.token), "cache-control": "no-store" },
  });
});
