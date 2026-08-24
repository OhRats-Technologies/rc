import type { User } from "../../../src/core";
import type { PasskeyView } from "../../../src/auth";
import type { ApiTokenView } from "../../../src/account";
import type { WorkspaceView } from "../../../src/workspaces";
import { htmlDocument } from "../document";
import { relative } from "../format";
import { SectionBadge } from "../components";

export function accountPage(user: User, workspaces: WorkspaceView[], passkeys: PasskeyView[], sidebar: "open" | "closed", error = "") {
  return htmlDocument({ title: "Account", user, workspaces, path: "/account", sidebar, scripts: ["account"], body:
    <div className="page"><header className="page-header"><div><p className="eyebrow">ACCOUNT</p><h1>{user.name}</h1></div></header>
      <section className="content-section"><div className="section-heading"><div><SectionBadge index="01">Passkeys</SectionBadge><h2>Sign-in credentials</h2></div><button id="add-passkey" className="text-button" type="button">ADD PASSKEY</button></div>
        <div className="settings-list">{passkeys.length ? passkeys.map((passkey, index) => <div className="setting-row" key={passkey.id}>
          <div><strong>Passkey {index + 1}</strong><div className="meta">{passkey.last_used ? `USED ${relative(passkey.last_used)}` : "NOT USED YET"}</div></div>
          {passkeys.length > 1 ? <form method="post" action={`/account/passkeys/${passkey.id}/delete`}><button className="text-button" type="submit">REMOVE</button></form>
            : <span className="meta" title="Add another passkey before removing this one.">LAST PASSKEY</span>}
        </div>) : <p className="empty-state">No passkeys. This browser session is your remaining access.</p>}</div><p id="passkey-error" className="error">{error}</p>
      </section>
    </div> });
}

export function apiPage(user: User, workspaces: WorkspaceView[], tokens: ApiTokenView[], sidebar: "open" | "closed", createdToken = "", error = "") {
  return htmlDocument({ title: "API access", user, workspaces, path: "/api", sidebar, scripts: createdToken ? ["copy"] : [], body:
    <div className="page"><header className="page-header"><div><p className="eyebrow">API</p><h1>API access</h1></div><a className="or-button" href="/api/v1/openapi">OPEN API DOCS <span aria-hidden="true">→</span></a></header>
      <section className="content-section"><div className="section-heading"><div><SectionBadge index="01">New token</SectionBadge><h2>Create credential</h2></div></div>
        <form method="post" action="/api/tokens" className="inline-form"><label>Name<input name="name" placeholder="Automation" required/></label><button className="or-button" type="submit">CREATE TOKEN</button></form>
        {createdToken && <div className="token-secret"><span className="meta">SHOWN ONCE</span><div className="or-copy-field" data-copy-value={createdToken} title={createdToken}><code>{createdToken}</code><button className="or-copy-button copy-value" type="button" aria-label="Copy API token"><span className="or-copy-icon" aria-hidden="true"/></button></div></div>}<p className="error">{error}</p>
      </section>
      <section className="content-section"><div className="section-heading"><div><SectionBadge index="02">Tokens</SectionBadge><h2>Active credentials</h2></div></div>
        <div className="settings-list">{tokens.length ? tokens.map(token => <div className="setting-row" key={token.id}><div><strong>{token.name}</strong><div className="meta">{token.last_used ? `USED ${relative(token.last_used)}` : "NEVER USED"}</div></div>
          <form method="post" action={`/api/tokens/${token.id}/delete`}><button className="text-button" type="submit">REVOKE</button></form></div>) : <p className="empty-state">No API tokens.</p>}</div>
      </section>
    </div> });
}
