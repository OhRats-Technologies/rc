import { PUBLIC_URL } from "../../../src/config";
import type { ReactNode } from "react";
import { htmlDocument } from "../document";
import { SectionBadge } from "../components";
import { Arrow, PublicFooter, PublicNav } from "../public";

function CopyField({ value, prefix = "$" }: { value: string; prefix?: string }) {
  return <div className="or-copy-field" data-copy-value={value} title={value}>{prefix && <span className="or-copy-prefix">{prefix}</span>}<code>{value}</code><button className="or-copy-button copy-value" type="button" aria-label="Copy"><span className="or-copy-icon" aria-hidden="true"/></button></div>;
}

function page(active: string, title: string, intro: string, children: ReactNode, copy = false) {
  return htmlDocument({ title, description: intro, canonicalPath: active === "docs" ? "/docs" : `/docs/${active}`, styles: ["public"], scripts: copy ? ["copy"] : [], indexable: true, publicSite: true, body:
    <div className="public-site"><PublicNav active={active}/><main className="container listing-page public-doc"><header className="section-header"><div className="section-title-container"><h1>{title}</h1><p className="listing-copy">{intro}</p></div></header><div className="public-doc-body">{children}</div></main><PublicFooter/></div>,
  });
}

function DocSection({ index, label, heading, children }: { index: string; label: string; heading: string; children: ReactNode }) {
  return <div className="public-doc-section"><div className="section-heading-stack"><SectionBadge index={index}>{label}</SectionBadge><div className="section-title-container"><h2>{heading}</h2></div></div><div className="public-doc-copy">{children}</div></div>;
}

export function docsPage() {
  return page("docs", "Docs", "Set up a passkey account, enroll a machine, and choose the control surface you need.", <>
    <DocSection index="01" label="Getting started" heading="Invite, passkey, enroll."><p>RC account creation is invitation-only. Open a workspace invite, create a passkey, then sign in.</p><p>From Devices, create an enrollment command and run it on macOS or Linux. The RC Node then reconnects outbound as a user service.</p><div className="public-doc-actions"><a className="header-link" href="/login">Sign in <Arrow/></a></div></DocSection>
    <DocSection index="02" label="Interfaces" heading="Choose the control surface."><p>The browser covers devices, terminals, Actions, access, and audit history. Use the CLI, API, or MCP when you need a different control surface.</p><div className="public-doc-actions"><a className="header-link" href="/docs/cli">CLI <Arrow/></a><a className="header-link" href="/docs/api">API <Arrow/></a><a className="header-link" href="/docs/mcp">MCP <Arrow/></a></div></DocSection>
    <DocSection index="03" label="Safety" heading="Authority stays verifiable."><p>Passkeys protect human authorization. RC Lock lets each Node verify workspace authority and execution grants.</p><p>Browser and CLI process traffic is encrypted client-to-Node. Node updates require a signed OhRats release.</p></DocSection>
  </>);
}

export function mcpDocsPage() {
  const endpoint = `${PUBLIC_URL}/mcp`, codex = `codex mcp add ohrats-rc --url ${endpoint} --oauth-resource ${endpoint}`;
  return page("mcp", "MCP", "Connect an AI agent to OhRats RC with scoped OAuth access.", <>
    <DocSection index="01" label="Setup" heading="Add OhRats RC to your client."><p>OhRats RC uses one shared MCP endpoint. The server advertises its display name as OhRats RC; clients that require a machine-safe configuration identifier can use <code>ohrats-rc</code>.</p><CopyField value={endpoint} prefix=""/><CopyField value={codex}/></DocSection>
    <DocSection index="02" label="Authorization" heading="Choose exactly what the agent can use."><p>OAuth opens RC in the browser. Choose machines, capabilities, and lifetime. Observe and Actions are separate permissions; Terminal is explicit and off by default.</p></DocSection>
    <DocSection index="03" label="Execution" heading="Node-verified execution."><p>Execution grants are signed by the passkey-backed control identity and pinned into RC Lock. Saved Actions are bound to the approved command and working directory.</p><p>MCP Terminal command and output pass through bounded RC server memory while a call is active. They are not persisted to SQLite process history.</p></DocSection>
  </>, true);
}

export function apiDocsPage() {
  return page("api", "API", "Proof-of-possession automation for OhRats RC.", <>
    <DocSection index="01" label="Keys" heading="Create a signing key."><p>Create API keys after passkey step-up. The Ed25519 private key is generated in the browser and shown once; RC stores only its public key, scopes, and expiry.</p><div className="public-doc-actions"><a className="header-link" href="/api">Manage API keys <Arrow/></a></div></DocSection>
    <DocSection index="02" label="Requests" heading="Sign every request."><p>Each request signs method, path and query, timestamp, nonce, and body hash. Replayed nonces are rejected; the Node independently enforces execution authority from RC Lock.</p></DocSection>
    <DocSection index="03" label="Reference" heading="OpenAPI reference."><p>The generated OpenAPI reference describes the public HTTP API and its proof-of-possession headers.</p><div className="public-doc-actions"><a className="header-link" href="/api/v1/openapi">Open API reference <Arrow/></a></div></DocSection>
  </>);
}

export function cliDocsPage() {
  return page("cli", "CLI", "Use the signed OhRats RC binary for human remote control from your terminal.", <>
    <DocSection index="01" label="Install" heading="Install the signed binary."><p>The same signed binary can operate as an enrolled RC Node and as the human CLI.</p><CopyField value={`curl -fsSL ${PUBLIC_URL}/install.sh | sh`}/></DocSection>
    <DocSection index="02" label="Sign in" heading="Authorize the CLI with a passkey."><p>CLI authorization opens the browser for passkey approval. CLI authorization defaults to until revoked; <code>--expires</code> accepts a finite lifetime when needed.</p><CopyField value="ohrats-rc login"/></DocSection>
    <DocSection index="03" label="Use" heading="Control your machines."><CopyField value="ohrats-rc devices"/><CopyField value="ohrats-rc shell DEVICE"/><CopyField value="ohrats-rc run DEVICE -- COMMAND"/></DocSection>
  </>, true);
}
