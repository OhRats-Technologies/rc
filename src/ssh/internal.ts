import { SSH_INTERNAL_PORT } from "../config";
import { sshKeyAuthorization, sshPrincipalForDevice, touchSshKey } from "../ssh-keys";
import { startSshProcess, type SshPrincipal } from "./process";

const SFTP_COMMAND = `if command -v sftp-server >/dev/null 2>&1; then exec "$(command -v sftp-server)"; elif [ -x /usr/lib/openssh/sftp-server ]; then exec /usr/lib/openssh/sftp-server; elif [ -x /usr/lib/ssh/sftp-server ]; then exec /usr/lib/ssh/sftp-server; elif [ -x /usr/libexec/sftp-server ]; then exec /usr/libexec/sftp-server; else echo 'sftp-server not installed' >&2; exit 127; fi`;

type BridgeData = {
  keyId: string;
  deviceId: string;
  process?: ReturnType<typeof startSshProcess>;
  started: boolean;
};

function shellCommand(original: string) {
  const value = original.trim();
  if (!value) return 'exec "${SHELL:-sh}" -l';
  if (value === "internal-sftp" || value.startsWith("internal-sftp ")) return SFTP_COMMAND;
  return original;
}

function authorized(request: Request) {
  const url = new URL(request.url), algorithm = url.searchParams.get("type") || "", keyData = url.searchParams.get("key") || "";
  if (!algorithm || !keyData) return new Response("", { status: 404 });
  const row = sshKeyAuthorization(algorithm, keyData);
  if (!row) return new Response("", { status: 404 });
  const keyId = String(row.id);
  const options = [
    "no-agent-forwarding", "no-port-forwarding", "no-X11-forwarding", "no-user-rc",
    `command="/usr/local/bin/rc-ssh-bridge ${keyId}"`,
  ].join(",");
  return new Response(`${options} ${row.algorithm} ${row.key_data}\n`, { headers: { "content-type": "text/plain" } });
}

function principal(keyId: string, deviceId: string): SshPrincipal | null {
  const row = sshPrincipalForDevice(keyId, deviceId);
  if (!row) return null;
  return {
    userId: String(row.user_id), keyId: String(row.key_id), deviceId: String(row.device_id),
    proof: { grant: String(row.grant), credentialId: String(row.credential_id), assertion: String(row.assertion) },
  };
}

export function startSshInternal() {
  const server = Bun.serve<BridgeData>({
    hostname: "127.0.0.1", port: SSH_INTERNAL_PORT,
    fetch(request, server) {
      const url = new URL(request.url);
      if (url.pathname === "/authorized") return authorized(request);
      if (url.pathname === "/bridge") {
        const keyId = url.searchParams.get("keyId") || "", deviceId = url.searchParams.get("deviceId") || "";
        if (!keyId || !deviceId || !principal(keyId, deviceId)) return new Response("forbidden", { status: 403 });
        if (server.upgrade(request, { data: { keyId, deviceId, started: false } })) return;
        return new Response("upgrade failed", { status: 500 });
      }
      return new Response("not found", { status: 404 });
    },
    websocket: {
      open(ws) { touchSshKey(ws.data.keyId); },
      message(ws, message) {
        if (!ws.data.started) {
          if (typeof message !== "string") return ws.close(1003, "start message required");
          let start: { type?: string; command?: string; terminal?: { cols?: number; rows?: number; term?: string } };
          try { start = JSON.parse(message); } catch { return ws.close(1003, "invalid start message"); }
          if (start.type !== "start") return ws.close(1003, "invalid start message");
          const auth = principal(ws.data.keyId, ws.data.deviceId);
          if (!auth) return ws.close(1008, "authorization revoked");
          const terminal = start.terminal ? {
            cols: Math.max(2, Math.min(500, Number(start.terminal.cols) || 80)),
            rows: Math.max(2, Math.min(500, Number(start.terminal.rows) || 24)),
            term: String(start.terminal.term || "xterm-256color").slice(0, 128),
          } : undefined;
          try {
            ws.data.process = startSshProcess(auth, shellCommand(String(start.command || "")), terminal, {
              stdout(data) { ws.send(Buffer.concat([Buffer.from([1]), data])); },
              stderr(data) { ws.send(Buffer.concat([Buffer.from([2]), data])); },
              exit(code, signal, error) {
                ws.send(JSON.stringify({ type: "exit", code, signal, ...(error ? { error } : {}) }));
                ws.close(1000, "process exited");
              },
            });
            ws.data.started = true;
          } catch (error) {
            ws.send(JSON.stringify({ type: "exit", code: 255, signal: "", error: error instanceof Error ? error.message : String(error) }));
            ws.close(1011, "process start failed");
          }
          return;
        }
        if (typeof message !== "string") {
          const bytes = Buffer.isBuffer(message) ? message : Buffer.from(message as ArrayBuffer);
          ws.data.process?.stdin(bytes); return;
        }
        let control: { type?: string; cols?: number; rows?: number; signal?: string };
        try { control = JSON.parse(message); } catch { return; }
        if (control.type === "stdin.close") ws.data.process?.closeStdin();
        if (control.type === "resize") ws.data.process?.resize(Math.max(2, Math.min(500, Number(control.cols) || 80)), Math.max(2, Math.min(500, Number(control.rows) || 24)));
        if (control.type === "signal") ws.data.process?.signal(String(control.signal || "").replace(/^SIG/, "").slice(0, 32));
      },
      close(ws) { ws.data.process?.kill(); },
    },
  });
  console.log(`RC SSH internal bridge listening on 127.0.0.1:${server.port}`);
  return server;
}
