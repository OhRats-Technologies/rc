export {};

const keyId = process.argv[2] || "", deviceId = process.env.RC_DEVICE_ID || "";
if (!keyId || !deviceId) {
  process.stderr.write("RC SSH target missing; regenerate your RC SSH config.\n");
  process.exit(255);
}
const port = process.env.RC_SSH_INTERNAL_PORT || "3001";
const terminal = Boolean(process.stdin.isTTY && process.stdout.isTTY);
if (terminal) (process.stdin as NodeJS.ReadStream).setRawMode?.(true);
const ws = new WebSocket(`ws://127.0.0.1:${port}/bridge?keyId=${encodeURIComponent(keyId)}&deviceId=${encodeURIComponent(deviceId)}`);
ws.binaryType = "arraybuffer";
let exited = false;
function size() {
  return { cols: Math.max(2, Number((process.stdout as NodeJS.WriteStream).columns) || 80), rows: Math.max(2, Number((process.stdout as NodeJS.WriteStream).rows) || 24) };
}
function send(value: unknown) { if (ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify(value)); }
ws.addEventListener("open", () => {
  send({ type: "start", command: process.env.SSH_ORIGINAL_COMMAND || "", ...(terminal ? { terminal: { ...size(), term: process.env.TERM || "xterm-256color" } } : {}) });
  process.stdin.on("data", chunk => { if (ws.readyState === WebSocket.OPEN) ws.send(chunk); });
  process.stdin.on("end", () => send({ type: "stdin.close" }));
});
ws.addEventListener("message", event => {
  if (typeof event.data === "string") {
    let message: { type?: string; code?: number; signal?: string; error?: string };
    try { message = JSON.parse(event.data); } catch { return; }
    if (message.type !== "exit") return;
    exited = true;
    if (message.error) process.stderr.write(`${message.error}\n`);
    process.exit(Math.max(0, Math.min(255, Number(message.code) || 0)));
  }
  const data = Buffer.from(event.data as ArrayBuffer);
  if (!data.length) return;
  (data[0] === 2 ? process.stderr : process.stdout).write(data.subarray(1));
});
ws.addEventListener("close", () => { if (!exited) process.exit(255); });
ws.addEventListener("error", () => { if (!exited) process.exit(255); });
process.on("SIGWINCH", () => { if (terminal) send({ type: "resize", ...size() }); });
for (const signal of ["SIGINT", "SIGTERM", "SIGQUIT", "SIGHUP"] as const) process.on(signal, () => send({ type: "signal", signal }));
