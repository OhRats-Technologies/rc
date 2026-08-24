import { htmlDocument } from "../document";

type AuthMode = "setup" | "login" | "register" | "join";

export function authPage(mode: AuthMode, options: { invite?: string; authorized?: boolean; error?: string } = {}) {
  const invite = options.invite || "", error = options.error || "";
  const title = mode === "setup" ? "Create Relay" : mode === "register" ? "Join Relay" : mode === "join" ? "Join workspace" : "Sign in";
  return htmlDocument({ title, scripts: mode === "join" ? [] : ["auth"], body:
    <section className="auth-shell"><div className="ohrats-grid auth-grid" aria-hidden="true"/><div className="auth-content">
      <p className="eyebrow">RELAY / CONTROL PLANE</p><h1>{title}</h1>
      {mode === "setup" && <><p className="muted">Create the first account with a passkey.</p>{options.authorized
        ? <form id="setup-form" className="auth-form"><label>Name<input name="name" autoComplete="name" required autoFocus/></label><button className="or-button" type="submit">CREATE PASSKEY</button></form>
        : <p className="page-copy">Open the setup link for this Relay instance.</p>}</>}
      {mode === "login" && <><p className="muted">Use a passkey or a full-access API token.</p>
        <form id="login-form" className="auth-form"><button className="or-button" type="submit">SIGN IN WITH PASSKEY</button></form><div className="auth-divider"><span>OR</span></div>
        <form method="post" action="/auth/token" className="auth-form compact-form"><input type="hidden" name="next" value={invite ? `/?invite=${encodeURIComponent(invite)}` : "/devices"}/><label>API token<input name="token" type="password" autoComplete="off" spellCheck={false} required/></label><button className="text-button auth-token-submit" type="submit">SIGN IN WITH TOKEN</button></form></>}
      {mode === "register" && <><p className="muted">Create a passkey to join this workspace.</p><form id="register-form" className="auth-form"><label>Name<input name="name" autoComplete="name" required/></label><input name="invite" type="hidden" value={invite}/><button className="or-button" type="submit">CREATE PASSKEY</button></form><a className="text-action" href={`/?invite=${encodeURIComponent(invite)}&signin=1`}>SIGN IN INSTEAD</a></>}
      {mode === "join" && <><p className="muted">Join this workspace with your current account.</p><form method="post" action="/workspaces/join" className="auth-form"><input type="hidden" name="token" value={invite}/><button className="or-button" type="submit">JOIN WORKSPACE</button></form></>}
      <p id="auth-error" className="error" role="alert">{error}</p>
    </div></section> });
}

export function notFoundPage(user?: import("../../../src/core").User, workspaces: import("../../../src/workspaces").WorkspaceView[] = [], sidebar: "open" | "closed" = "open") {
  return htmlDocument({ title: "Not found", user, workspaces, path: "", sidebar, status: 404, body:<div className="page"><header className="page-header"><div><p className="eyebrow">404</p><h1>Not found</h1></div></header></div> }, !!user);
}
