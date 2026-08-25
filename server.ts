import "reflect-metadata";
import { app } from "./src/app";
import { PORT, PUBLIC_URL } from "./src/config";
import { now, q } from "./src/db";
import { recoverInterruptedProcesses } from "./src/gateway";

recoverInterruptedProcesses();

app.listen({
  port: PORT,
  hostname: "0.0.0.0",
  idleTimeout: 60,
  maxRequestBodySize: 1024 * 1024,
  development: Bun.env.NODE_ENV === "development",
});

setInterval(() => {
  q("DELETE FROM auth_sessions WHERE expires_at<?").run(now());
  q("DELETE FROM step_up_authorizations WHERE expires_at<?").run(now());
  q("DELETE FROM step_up_tokens WHERE expires_at<?").run(now());
  q("DELETE FROM workspace_invites WHERE expires_at<? AND used_at IS NULL").run(now());
  q("DELETE FROM enrollment_tokens WHERE expires_at<? AND used_at IS NULL").run(now());
  q("DELETE FROM webauthn_challenges WHERE expires_at<?").run(now());
  q("DELETE FROM cli_authorizations WHERE expires_at<? OR exchanged_at IS NOT NULL").run(now());
  q("DELETE FROM cli_sessions WHERE expires_at<?").run(now());
  q("DELETE FROM agent_auth_challenges WHERE expires_at<?").run(now());
}, 60_000).unref();

console.log(`RC ${PUBLIC_URL} listening on :${app.server?.port || PORT}`);
