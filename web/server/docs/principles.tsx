import type { DocArticle } from "./types";

export function principlesArticle(): DocArticle {
  return {
    slug: "principles",
    title: "Principles",
    intro: "The constraints RC uses when deciding where trust and execution authority belong.",
    sections: [
      {
        id: "no-inbound-port",
        title: "No inbound device port",
        body: <>
          <p>The RC Node creates an outbound authenticated WebSocket connection to RC. A controlled machine does not need a public SSH port, port-forward, or inbound RC listener.</p>
          <p>Browser, CLI, API, and MCP requests are routed through the hosted service to the already-connected Node.</p>
        </>,
      },
      {
        id: "node-authority",
        title: "The Node enforces execution authority",
        body: <>
          <p>Workspace membership in the hosted database is not sufficient to execute on a locked Node. Each Node keeps an RC Lock authority snapshot locally.</p>
          <p>After the initial lock bootstrap, authority changes require a valid Owner-authorized transition from the Node's current lock generation and hash.</p>
        </>,
      },
      {
        id: "human-auth",
        title: "Human authentication uses passkeys",
        body: <>
          <p>Account creation, browser login, sensitive step-up operations, CLI authorization, and execution-capable MCP approval use WebAuthn passkeys. RC does not store account passwords.</p>
        </>,
      },
      {
        id: "automation-auth",
        title: "Automation is scoped and cryptographic",
        body: <>
          <p>API keys are Ed25519 proof-of-possession credentials. RC stores the public key; every request signs its method, path/query, timestamp, nonce, and body hash.</p>
          <p>MCP grants select specific machines and scopes. Execution-capable grants are signed by a passkey-backed control identity and included in RC Lock.</p>
        </>,
      },
      {
        id: "encrypted-control",
        title: "Browser and CLI control is end-to-end encrypted",
        body: <>
          <p>Terminal commands, input, output, signals, and lifecycle messages use a client-to-Node AES-256-GCM session key derived from fresh and pinned X25519 material.</p>
          <p>After authorization, browser and CLI clients prefer a direct WebRTC DataChannel to the Node. The hosted service performs signaling. If direct ICE cannot establish a DataChannel, the same encrypted control frames fall back to the hosted WebSocket relay.</p>
          <p>Standard remote MCP does not use this custom transport; MCP Terminal behavior is documented separately in the <a href="/docs/security#mcp-terminal">security model</a>.</p>
        </>,
      },
      {
        id: "signed-updates",
        title: "Node updates are release-signed",
        body: <>
          <p>The Node embeds an OhRats Ed25519 release public key. Updates require a signed release manifest, an artifact SHA-256 match, and a version that is not a downgrade.</p>
          <p>The release private key is not stored in the RC runtime or repository.</p>
        </>,
      },
    ],
  };
}
