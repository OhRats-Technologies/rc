import { CodeBlock, CopyField, DocTable } from "./components";
import type { DocArticle } from "./types";

export function apiArticle(): DocArticle {
  const canonical = `rc-api-v1
<key-id>
<unix-timestamp-seconds>
<nonce>
<HTTP-METHOD>
<path-and-query>
<hex-sha256-body>`;
  return {
    slug: "api",
    title: "API",
    intro: "Create proof-of-possession API keys, sign requests, and use the OpenAPI reference.",
    copy: true,
    sections: [
      {
        id: "create-key",
        title: "Create an API key",
        body: <>
          <p>Sign in and open <a href="/api">API keys</a>. Create a key, choose scopes and lifetime, and complete passkey step-up.</p>
          <p>The browser generates an Ed25519 key pair and sends RC only the public key. The private key is shown once as <code>rcsk_&lt;key-id&gt;_&lt;pkcs8-private-key-base64url&gt;</code>. RC cannot recover it later.</p>
          <p>API keys default to until revoked. Finite lifetimes are 1 hour, 1 day, 7 days, 30 days, 90 days, 180 days, and 1 year.</p>
        </>,
      },
      {
        id: "scopes",
        title: "Scopes",
        body: <>
          <DocTable><thead><tr><th>Scope</th><th>Allows</th></tr></thead><tbody>
            <tr><td><code>read</code></td><td>GET requests to account, workspace, device, Action, and process resources.</td></tr>
            <tr><td><code>execute</code></td><td>Allocate device processes and run saved Actions.</td></tr>
            <tr><td><code>manage-devices</code></td><td>Create enrollments and modify or remove devices.</td></tr>
            <tr><td><code>manage-workspaces</code></td><td>Create or modify workspaces and Actions.</td></tr>
          </tbody></DocTable>
          <p>Passkey and API-key administration require a human browser session and cannot be performed with an API key.</p>
        </>,
      },
      {
        id: "request-headers",
        title: "Request headers",
        body: <>
          <DocTable><tbody>
            <tr><th><code>X-RC-Key-ID</code></th><td>The key ID from the <code>rcsk_</code> credential.</td></tr>
            <tr><th><code>X-RC-Timestamp</code></th><td>Current Unix time in seconds. RC accepts a 60-second clock window.</td></tr>
            <tr><th><code>X-RC-Nonce</code></th><td>A new random value for every request. Reuse is rejected.</td></tr>
            <tr><th><code>X-RC-Signature</code></th><td>Base64url Ed25519 signature of the canonical payload.</td></tr>
          </tbody></DocTable>
        </>,
      },
      {
        id: "canonical-payload",
        title: "Canonical payload",
        body: <>
          <CodeBlock>{canonical}</CodeBlock>
          <p><code>&lt;path-and-query&gt;</code> is the request URI beginning with <code>/</code>. <code>&lt;hex-sha256-body&gt;</code> is the lowercase hexadecimal SHA-256 digest of the exact request body bytes. Hash zero bytes for an empty body.</p>
          <p>The signature is Ed25519 over the UTF-8 bytes of that newline-delimited payload.</p>
        </>,
      },
      {
        id: "cli",
        title: "Use an API key with the RC CLI",
        body: <>
          <CopyField value="RC_API_TOKEN='rcsk_...' ohrats-rc devices"/>
          <CopyField value="ohrats-rc devices --token 'rcsk_...'"/>
          <p>The CLI parses the private key from the <code>rcsk_</code> credential and signs each request locally. It does not transmit the private key as a bearer token.</p>
        </>,
      },
      {
        id: "openapi",
        title: "OpenAPI reference",
        body: <>
          <p>The generated OpenAPI schema contains the public API routes, request/response schemas, and proof-of-possession security scheme.</p>
          <ul>
            <li><a href="/api/v1/openapi">OpenAPI reference</a></li>
            <li><a href="/api/v1/openapi/json">OpenAPI JSON</a></li>
          </ul>
          <p>Authenticated requests still require a valid RC proof-of-possession signature. The reference does not receive or store your API private key.</p>
        </>,
      },
    ],
  };
}
