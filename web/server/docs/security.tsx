import type { DocArticle } from "./types";

export function securityArticle(): DocArticle {
  return {
    slug: "security",
    title: "Security model",
    intro: "Trust boundaries, encrypted control, RC Lock, Node authentication, data handling, and Node releases.",
    sections: [
      {
        id: "trust-boundaries",
        title: "Trust boundaries",
        body: <>
          <p>The hosted RC service authenticates users, coordinates workspaces, relays live connections, and stores control-plane metadata. The Node is the final execution verifier for locked machines.</p>
          <p>The browser application is part of the trusted client computing base because RC serves its JavaScript. A fully compromised web server could replace that JavaScript when an authorized user visits and ask the browser-held control key to sign operations. RC Lock prevents the server/database from creating those signatures by itself; the standalone CLI avoids this web-code-delivery dependency.</p>
          <p>RC does not claim that browser control is resistant to a malicious server actively serving modified client code.</p>
        </>,
      },
      {
        id: "encrypted-control",
        title: "Browser and CLI encrypted control",
        body: <>
          <p>Each live browser or CLI control session creates a fresh client X25519 key pair. The Node has a pinned static X25519 transport key and creates a fresh X25519 key pair for the session.</p>
          <ol>
            <li>The client and Node derive one X25519 secret using the Node static transport key.</li>
            <li>They derive a second X25519 secret using the fresh Node ephemeral key.</li>
            <li>The two secrets are concatenated and passed through HKDF-SHA256.</li>
            <li>The derivation is salted with the session challenge and bound to the device ID and client ID.</li>
            <li>The result is an AES-256-GCM key used for process control traffic.</li>
          </ol>
          <p>The Node signs the handshake with its Ed25519 device identity. The signature binds the challenge, device, client, client ephemeral key, Node static transport key, Node ephemeral key, and session ID.</p>
          <p>Frames use direction-specific nonces and authenticated additional data containing the session ID, sequence number, and direction. Sequence numbers must increase by exactly one. A gap or replay closes the session.</p>
          <p>A later compromise of only the Node's long-term X25519 transport key is not enough to decrypt a recorded session because the fresh Node ephemeral private key is not retained.</p>
        </>,
      },
      {
        id: "control-transport",
        title: "Control transport",
        body: <>
          <p>The authenticated RC WebSocket carries control signaling and hosted live events. After the encrypted control session is authorized, browser and CLI clients try to move its opaque encrypted frames onto a reliable ordered WebRTC DataChannel.</p>
          <p>The managed service currently uses <code>stun.cloudflare.com:3478</code> for ICE candidate discovery. While the direct DataChannel is available, terminal data does not traverse the RC application server. If the direct transport cannot be established or later fails, the same encrypted session continues over the WebSocket relay.</p>
          <p>WebRTC adds DTLS transport encryption, but RC currently retains its own AES-256-GCM control framing inside the DataChannel. Changing or removing that inner encryption is a separate security change.</p>
        </>,
      },
      {
        id: "rc-lock",
        title: "RC Lock",
        body: <>
          <p>RC Lock is the authority snapshot stored locally on each Node. It contains workspace members and roles, passkey credential public keys, authorized API signing public keys with scopes and expiry, and hashes of active execution-capable MCP grants.</p>
          <p>The first lock is a trust-on-first-use bootstrap from the RC instance used during enrollment. It never overwrites an existing lock.</p>
          <p>Every later authority change must reference the Node's current lock hash and generation and be signed by a passkey-backed control identity belonging to a user who is already an Owner in the current snapshot.</p>
          <p>The Node rejects stale transitions, invalid signatures, expired API/MCP credentials, and credentials missing from the local snapshot. Accepting a new lock generation invalidates live encrypted control sessions so they reconnect against current authority.</p>
        </>,
      },
      {
        id: "node-authentication",
        title: "Node authentication",
        body: <>
          <p>Node HTTP and WebSocket authentication uses the Node's Ed25519 identity. RC issues a short-lived one-time challenge; the Node signs the challenge together with the request method and path.</p>
          <p>Consumed challenges cannot be reused. Current Nodes also reject legacy plaintext browser/CLI process, update, and remove commands.</p>
        </>,
      },
      {
        id: "process-data",
        title: "Process data",
        body: <>
          <p>SQLite stores process metadata such as process ID, device, state, timestamps, exit code, and signal.</p>
          <p>Browser and CLI process command, working directory, stdin, stdout, and stderr are not persisted in process history. Direct WebRTC sessions bypass the application server; relay fallback carries only encrypted control frames.</p>
        </>,
      },
      {
        id: "mcp-terminal",
        title: "MCP Terminal",
        body: <>
          <p>Standard remote MCP does not participate in RC's browser/CLI end-to-end transport. For <code>mcp:terminal</code>, command and output plaintext pass through RC server memory while the MCP call is active.</p>
          <p>That memory is bounded to 256 KiB per active process buffer. Completed buffers expire after five minutes; inactive running buffers expire after thirty minutes. MCP command/output is not written to SQLite process history.</p>
          <p>A compromised RC server with an active Terminal grant could issue commands allowed by that grant until the grant expires or is revoked.</p>
        </>,
      },
      {
        id: "ssh-gateway",
        title: "SSH gateway",
        body: <>
          <p>RC runs stock OpenSSH <code>sshd</code> at the gateway. Clients normally reach it through <code>rc ssh-proxy</code> over the public HTTPS/WebSocket endpoint; Nodes keep only their outbound RC connection and do not expose SSH.</p>
          <p>Password and keyboard-interactive authentication are disabled. A registered SSH public key is bound to a passkey-backed RC control identity, and the forced bridge routes by the immutable RC device ID. The Node independently verifies that control grant and requires the user to be an Operator or Owner before starting a process.</p>
          <p>SSH terminates at the RC gateway, so commands, terminal bytes, file contents, and rsync traffic are plaintext in gateway memory while active. A compromised gateway with an active SSH authorization could act as that user. This is a deliberate trust model difference from browser/CLI end-to-end control.</p>
        </>,
      },
      {
        id: "node-updates",
        title: "Node release integrity",
        body: <>
          <p>GitHub Releases are the Node release trust boundary. The updater reads the published release manifest, verifies the selected artifact SHA-256, verifies the downloaded binary's reported version, and refuses downgrades.</p>
          <p>Node releases are published independently of RC runtime deployments, so a normal control-plane deploy does not rebuild or replace Node binaries.</p>
        </>,
      },
    ],
  };
}
