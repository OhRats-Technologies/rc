import { DocTable } from "./components";
import type { DocArticle } from "./types";

export function authenticationArticle(): DocArticle {
  return {
    slug: "authentication",
    title: "Authentication",
    intro: "Passkeys, control identities, CLI sessions, API keys, MCP OAuth, lifetimes, and revocation.",
    sections: [
      {
        id: "passkeys",
        title: "Passkeys",
        body: <>
          <p>Passkeys are the primary human credential. Account creation registers a WebAuthn credential; browser login verifies a WebAuthn assertion.</p>
          <p>The OhRats-managed service may allow public signup. Public signup is gated by Cloudflare Turnstile before RC issues a WebAuthn registration ceremony. Self-hosted instances remain invite/setup-only unless their operator explicitly enables and configures public signup.</p>
          <p>Sensitive operations can require a fresh passkey assertion even when a browser session is already active. Examples include authority changes and durable execution authorization.</p>
        </>,
      },
      {
        id: "browser-sessions",
        title: "Browser sessions",
        body: <>
          <p>Browser login creates an HTTP-only RC session cookie. The default lifetime is 30 days.</p>
          <p>Available lifetimes are 1 hour, 1 day, 7 days, 30 days, 90 days, 180 days, and 1 year. Browser sessions do not offer an until-revoked cookie lifetime.</p>
        </>,
      },
      {
        id: "control-identities",
        title: "Control identities",
        body: <>
          <p>The first browser control action creates a local Ed25519 control identity. RC asks a passkey to authorize that public key for the requested control lifetime.</p>
          <p>The browser retains the signing key locally. RC stores the public key and passkey-backed authorization needed for the Node to verify later control operations.</p>
        </>,
      },
      {
        id: "cli-authentication",
        title: "CLI authentication",
        body: <>
          <p><code>rc login</code> generates a local Ed25519 control key, starts a short browser authorization flow, and waits for passkey approval.</p>
          <p>The default CLI authorization lifetime is until revoked. Finite values are <code>1h</code>, <code>1d</code>, <code>7d</code>, <code>30d</code>, <code>90d</code>, <code>180d</code>, and <code>1y</code>.</p>
          <p><code>rc logout</code> revokes the hosted CLI session and deletes the local account session.</p>
        </>,
      },
      {
        id: "api-keys",
        title: "API keys",
        body: <>
          <p>API keys are Ed25519 proof-of-possession credentials. The browser generates the key pair and sends RC only the public key. The private key is shown once in the <code>rcsk_</code> credential.</p>
          <p>Every API request signs the key ID, timestamp, nonce, method, path/query, and SHA-256 body digest. RC accepts a 60-second timestamp window and rejects reused nonces.</p>
          <p>API keys default to until revoked and may be scoped to <code>read</code>, <code>execute</code>, <code>manage-devices</code>, and <code>manage-workspaces</code>.</p>
        </>,
      },
      {
        id: "mcp-oauth",
        title: "MCP OAuth",
        body: <>
          <p>MCP uses OAuth with Protected Resource Metadata, PKCE S256, and an exact resource value for the RC MCP endpoint.</p>
          <p>Access tokens are short-lived and refresh tokens rotate. The durable MCP grant separately records selected machines, scopes, and authorization lifetime.</p>
          <p>Execution-capable approval requires an Owner and a fresh passkey ceremony. A browser control identity signs the exact grant, and its hash is synchronized into RC Lock before the grant is usable on a Node.</p>
        </>,
      },
      {
        id: "lifetimes",
        title: "Default lifetimes",
        body: <>
          <DocTable><thead><tr><th>Authorization</th><th>Default</th></tr></thead><tbody>
            <tr><td>Web session</td><td>30 days</td></tr>
            <tr><td>CLI session</td><td>Until revoked</td></tr>
            <tr><td>MCP grant</td><td>Until revoked</td></tr>
            <tr><td>API key</td><td>Until revoked</td></tr>
            <tr><td>MCP access token</td><td>15 minutes</td></tr>
          </tbody></DocTable>
          <p>Protocol challenges, OAuth authorization codes, WebAuthn ceremonies, and step-up tokens remain short-lived independently of the durable authorization lifetime.</p>
        </>,
      },
      {
        id: "revocation",
        title: "Revocation",
        body: <>
          <ul>
            <li>Browser logout invalidates the browser session.</li>
            <li><code>rc logout</code> revokes the CLI session.</li>
            <li>Revoking an API key removes it from hosted state and synchronizes affected RC Locks.</li>
            <li>Revoking an MCP connection invalidates its OAuth tokens and removes its execution grant from affected RC Locks.</li>
            <li>Removing a passkey prevents future assertions with that credential. RC prevents removing the last viable primary credential through the normal UI/API path.</li>
          </ul>
        </>,
      },
    ],
  };
}
