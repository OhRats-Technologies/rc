import { PUBLIC_URL } from "../../../src/config";
import { htmlDocument } from "../document";
import { SectionBadge } from "../components";

const mcpEndpoint = `${PUBLIC_URL}/mcp`;
const codexSetup = `codex mcp add ohrats-rc --url ${mcpEndpoint} --oauth-resource ${mcpEndpoint}`;
const cliInstall = `curl -fsSL ${PUBLIC_URL}/install.sh | sh`;

function CopyField({ value, prefix = "$" }: { value: string; prefix?: string }) {
  return <div className="or-copy-field landing-copy" data-copy-value={value} title={value}>
    {prefix && <span className="or-copy-prefix">{prefix}</span>}<code>{value}</code>
    <button className="or-copy-button copy-value" type="button" aria-label="Copy"><span className="or-copy-icon" aria-hidden="true"/></button>
  </div>;
}

export function landingPage() {
  return htmlDocument({ title: "Remote control for your machines", styles: ["landing"], scripts: ["landing", "copy"], indexable: true, body:
    <div className="landing-shell">
      <nav className="landing-nav" aria-label="Primary">
        <a className="landing-brand" href="/" aria-label="OhRats RC home">
          <img src="https://assets.ohrats.party/assets/logo.092a1cece4d0.svg" alt=""/><strong>OhRats RC</strong>
        </a>
        <div className="landing-nav-tabs" aria-label="Resources">
          <a href="#docs">Docs</a><a href="#mcp">MCP</a><a href="#api">API</a><a href="#cli">CLI</a>
        </div>
        <div className="landing-nav-actions">
          <button className="theme-toggle" data-theme-toggle aria-label="Toggle theme"/>
          <a className="landing-signin" href="/login">Sign in</a>
          <a className="or-button" href="#get-started">Get started <span aria-hidden="true">→</span></a>
        </div>
      </nav>

      <main>
        <header className="landing-hero">
          <div className="ohrats-grid landing-grid" aria-hidden="true"/>
          <div className="landing-hero-content">
            <p className="eyebrow">OHRATS RC / REMOTE CONTROL</p>
            <h1>Control your machines<br/><span>without exposing SSH.</span></h1>
            <p>Persistent terminals, saved Actions, CLI control, signed automation, and scoped AI-agent access from one private control plane.</p>
            <div className="landing-hero-actions"><a className="or-button" href="#get-started">GET STARTED <span aria-hidden="true">→</span></a><a className="landing-text-link" href="/login">SIGN IN</a></div>
          </div>
        </header>

        <section className="landing-section" id="safety">
          <div className="landing-container">
            <header className="landing-section-heading"><div><SectionBadge index="01">Safety stack</SectionBadge><h2>Remote access with<br/><span>multiple independent checks.</span></h2></div></header>
            <div className="safety-grid">
              <article><span>01</span><h3>Passkeys, not passwords</h3><p>Human login and sensitive authorization use WebAuthn with user verification. RC does not use account passwords.</p></article>
              <article><span>02</span><h3>RC Lock on every Node</h3><p>Workspace authority is stored locally on the Node. Owners sign authority transitions so changing the hosted database alone cannot invent a new execution identity.</p></article>
              <article><span>03</span><h3>Encrypted browser + CLI control</h3><p>Interactive browser and CLI control use authenticated client/Node key exchange and encrypted process frames. Hosted process history keeps metadata, not terminal plaintext.</p></article>
              <article><span>04</span><h3>Proof-of-possession API keys</h3><p>API automation signs the method, path, timestamp, nonce, and body digest with Ed25519. Replayed nonces are rejected and scopes are explicit.</p></article>
              <article><span>05</span><h3>Scoped MCP authorization</h3><p>AI agents use OAuth, short-lived access tokens, selected machines, and explicit Observe / Actions / Terminal scopes. Execution-capable grants are pinned into RC Lock.</p></article>
              <article><span>06</span><h3>Signed Node releases</h3><p>The Node verifies an OhRats release signature and artifact hash before updating, and refuses signed downgrades.</p></article>
            </div>
            <p className="landing-caveat">MCP Terminal is intentionally different from browser/CLI control: standard remote MCP command/output plaintext passes through bounded RC server memory, but is not persisted to SQLite process history.</p>
          </div>
        </section>

        <section className="landing-section landing-resources" id="docs">
          <div className="landing-container">
            <header className="landing-section-heading"><div><SectionBadge index="02">Resources</SectionBadge><h2>One control plane.<br/><span>Four ways in.</span></h2></div></header>
            <div className="resource-tabs" role="tablist" aria-label="OhRats RC resources">
              <button type="button" role="tab" aria-selected="true" data-resource-tab="docs">Docs</button>
              <button type="button" role="tab" aria-selected="false" data-resource-tab="mcp">MCP</button>
              <button type="button" role="tab" aria-selected="false" data-resource-tab="api">API</button>
              <button type="button" role="tab" aria-selected="false" data-resource-tab="cli">CLI</button>
            </div>
            <div className="resource-panels">
              <article role="tabpanel" data-resource-panel="docs">
                <p className="eyebrow">QUICK START</p><h3>Invite → passkey → enroll.</h3>
                <ol><li>Create your account from a workspace invite link.</li><li>Open Devices and create a one-time enrollment command.</li><li>Run the command on macOS or Linux; the RC Node stays connected as a user background service.</li><li>Use terminals, saved Actions, the CLI, API keys, or MCP from the same workspace.</li></ol>
              </article>
              <article role="tabpanel" data-resource-panel="mcp" hidden id="mcp">
                <p className="eyebrow">MODEL CONTEXT PROTOCOL</p><h3>Connect an AI agent to OhRats RC.</h3>
                <p>Use the shared MCP endpoint. The server advertises itself as <strong>OhRats RC</strong>; clients that require a configuration identifier can use <code>ohrats-rc</code>.</p>
                <CopyField value={codexSetup}/><p className="resource-note">Codex will discover RC OAuth and ask you to choose machines, capabilities, and access duration in the browser.</p>
              </article>
              <article role="tabpanel" data-resource-panel="api" hidden id="api">
                <p className="eyebrow">HTTP API</p><h3>Signed automation without bearer API secrets.</h3>
                <p>Create an API signing key after passkey step-up. RC stores only its public key; requests use proof-of-possession signatures and scoped permissions.</p>
                <a className="landing-inline-cta" href="/api/v1/openapi">OPEN API REFERENCE <span aria-hidden="true">→</span></a>
              </article>
              <article role="tabpanel" data-resource-panel="cli" hidden id="cli">
                <p className="eyebrow">COMMAND LINE</p><h3>Install once, then sign in with a passkey.</h3>
                <CopyField value={cliInstall}/><CopyField value="ohrats-rc login"/>
                <p className="resource-note">The same signed binary can run as a device Node or as the human RC CLI.</p>
              </article>
            </div>
          </div>
        </section>

        <section className="landing-section landing-start" id="get-started">
          <div className="landing-container landing-start-grid">
            <div><SectionBadge index="03">Get started</SectionBadge><h2>Create an account<br/><span>or come back in.</span></h2><p>RC keeps account creation invite-based. A workspace owner gives you an invite; opening it starts passkey registration immediately.</p></div>
            <div className="landing-start-actions">
              <form className="landing-invite-form" data-invite-start><label>Workspace invite<input name="invite" placeholder="Paste invite URL or token" autoComplete="off" required/></label><button className="or-button" type="submit">CREATE ACCOUNT <span aria-hidden="true">→</span></button></form>
              <div className="landing-start-divider"><span>OR</span></div>
              <a className="landing-inline-cta" href="/login">SIGN IN WITH PASSKEY <span aria-hidden="true">→</span></a>
              <p>Account creation requires a workspace invitation. Registration uses a passkey; no email address or password is required.</p>
            </div>
          </div>
        </section>
      </main>

      <footer className="landing-footer"><a href="https://ohrats.party/">OhRats Technologies</a><span>Remote control for your machines.</span><span>© 2026 OhRats Technologies</span></footer>
    </div>
  });
}
