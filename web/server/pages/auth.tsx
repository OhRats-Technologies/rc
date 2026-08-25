import { htmlDocument } from "../document";

type AuthMode = "setup" | "login" | "register" | "join" | "invalid-invite";

export function authPage(mode: AuthMode, options: {
  invite?: string; authorized?: boolean; error?: string; workspaceName?: string; role?: "operator" | "viewer"; next?: string;
} = {}) {
  const invite = options.invite || "", error = options.error || "";
  const joinTitle = options.workspaceName ? `Join ${options.workspaceName}` : "Join workspace";
  const title = mode === "setup" ? "Create RC" : mode === "register" || mode === "join" ? joinTitle
    : mode === "invalid-invite" ? "Invite unavailable" : "Sign in";
  return htmlDocument({ title, scripts: mode === "join" || mode === "invalid-invite" ? [] : ["auth"], body:
    <section className="auth-shell"><div className="ohrats-grid auth-grid" aria-hidden="true"/><div className="auth-content" data-auth-next={options.next || ""}>
      <p className="eyebrow">OHRATS RC / REMOTE CONTROL</p><h1>{title}</h1>
      {mode === "setup" && <><p className="muted">Create the first account with a passkey.</p>{options.authorized
        ? <form id="setup-form" className="auth-form"><label>Name<input name="name" autoComplete="name" required autoFocus/></label><button className="or-button" type="submit">CREATE PASSKEY</button></form>
        : <p className="page-copy">Open the setup link for this RC instance.</p>}</>}
      {mode === "login" && <><p className="muted">Use your passkey to sign in.</p>
        <form id="login-form" className="auth-form"><button className="or-button" type="submit">SIGN IN WITH PASSKEY</button></form><p className="muted">New accounts are created from workspace invite links.</p></>}
      {mode === "register" && <><p className="muted">Create a passkey to join as {options.role || "operator"}.</p><form id="register-form" className="auth-form"><label>Name<input name="name" autoComplete="name" required/></label><input name="invite" type="hidden" value={invite}/><button className="or-button" type="submit">CREATE PASSKEY</button></form><a className="text-action" href={`/?invite=${encodeURIComponent(invite)}&signin=1`}>SIGN IN INSTEAD</a></>}
      {mode === "join" && <><p className="muted">Join as {options.role || "operator"} with your current account.</p><form method="post" action="/workspaces/join" className="auth-form"><input type="hidden" name="token" value={invite}/><button className="or-button" type="submit">JOIN WORKSPACE</button></form></>}
      {mode === "invalid-invite" && <><p className="muted">This workspace invite is invalid, expired, or already used.</p><a className="text-action" href="/">SIGN IN</a></>}
      <p id="auth-error" className="error" role="alert">{error}</p>
    </div></section> });
}

export function cliLoginPage(user: import("../../../src/core").User, code: string, approved = false, error = "", clientId = "", signingPublicKey = "") {
  return htmlDocument({ title: approved ? "CLI authorized" : "Authorize CLI", scripts: approved ? [] : ["cli-authorize"], body:
    <section className="auth-shell" data-cli-client={clientId} data-cli-public-key={signingPublicKey}><div className="ohrats-grid auth-grid" aria-hidden="true"/><div className="auth-content">
      <p className="eyebrow">OHRATS RC / COMMAND LINE</p><h1>{approved ? "CLI authorized" : "Authorize CLI"}</h1>
      {approved ? <p className="page-copy">Return to your terminal. This tab can be closed.</p> : <>
        <p className="page-copy">Allow the RC command line to act as <strong>{user.name}</strong> on your workspaces.</p>
        <form method="post" action="/cli/login" className="auth-form"><input type="hidden" name="code" value={code}/><button className="or-button" type="submit">AUTHORIZE CLI</button></form>
      </>}
      <p className="error" role="alert">{error}</p>
    </div></section> });
}

export function notFoundPage(user?: import("../../../src/core").User, workspaces: import("../../../src/workspaces").WorkspaceView[] = [], sidebar: "open" | "closed" = "open") {
  return htmlDocument({ title: "Not found", user, workspaces, path: "", sidebar, status: 404, body:<div className="page"><header className="page-header"><div><p className="eyebrow">404</p><h1>Not found</h1></div></header></div> }, !!user);
}
