import { mkdtemp, mkdir, rm, copyFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { createServer } from "node:net";
import type { AddressInfo } from "node:net";

const root = resolve(import.meta.dir, "..");
const kernel = process.env.RC_E2E_KERNEL || join(root, "kernel/target/debug/rc-kernel");
const cdp = process.env.RC_CDP_URL || "http://127.0.0.1:9223";
const artifacts = ["identity-http", "identity-store", "webauthn-es256", "webui-shell"];

function sleep(ms: number) { return new Promise(resolve => setTimeout(resolve, ms)); }
async function freePort() {
  return await new Promise<number>((resolvePort, reject) => {
    const server = createServer();
    server.once("error", reject);
    server.listen(0, "127.0.0.1", () => {
      const port = (server.address() as AddressInfo).port;
      server.close(error => error ? reject(error) : resolvePort(port));
    });
  });
}
async function waitForHttp(url: string, timeoutMs = 20_000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    try { if ((await fetch(url)).ok) return; } catch {}
    await sleep(100);
  }
  throw new Error(`HTTP endpoint did not become ready: ${url}`);
}
function run(args: string[]) {
  const result = Bun.spawnSync([kernel, ...args], { stdout: "pipe", stderr: "pipe" });
  if (result.exitCode !== 0) throw new Error(`${args.join(" ")} failed: ${result.stderr.toString()}`);
}

const directory = await mkdtemp(join(tmpdir(), "rc-identity-http-"));
const components = join(directory, "components");
await mkdir(components);
for (const artifact of artifacts) {
  const source = join(root, "dist/components", `${artifact}.wasm`);
  if (!await Bun.file(source).exists()) throw new Error(`missing ${source}`);
  await copyFile(source, join(components, `${artifact}.wasm`));
}
const port = await freePort(), base = `http://localhost:${port}`;
const setupToken = `identity-e2e-${crypto.randomUUID()}`;
const componentArgs = ["--component-dir", components];
run([...componentArgs, "identity-config", "public-url", base]);
run([...componentArgs, "identity-config", "setup-token", setupToken]);
run([...componentArgs, "webui-config", "public-url", base]);
let server: ReturnType<typeof Bun.spawn> | null = null;
let target: { id: string; webSocketDebuggerUrl: string } | null = null;
let socket: WebSocket | null = null;

function startServer() {
  server = Bun.spawn([kernel, ...componentArgs, "serve", "--listen", `127.0.0.1:${port}`], {
    stdout: "inherit", stderr: "inherit",
  });
}
async function stopServer() {
  if (!server) return;
  server.kill("SIGTERM");
  await Promise.race([server.exited, sleep(3000)]).catch(() => {});
  server = null;
}

try {
  startServer(); await waitForHttp(`${base}/healthz`);
  target = await fetch(`${cdp}/json/new?${encodeURIComponent("about:blank")}`, { method: "PUT" })
    .then(async response => {
      if (!response.ok) throw new Error(`create Chrome target: ${response.status}`);
      return await response.json() as { id: string; webSocketDebuggerUrl: string };
    });
  socket = new WebSocket(target.webSocketDebuggerUrl);
  let sequence = 0;
  const pending = new Map<number, { resolve(value: any): void; reject(error: Error): void }>();
  socket.onmessage = event => {
    const message = JSON.parse(String(event.data));
    const waiter = pending.get(message.id); if (!waiter) return;
    pending.delete(message.id);
    if (message.error) waiter.reject(new Error(message.error.message));
    else waiter.resolve(message.result || {});
  };
  await new Promise<void>((resolveSocket, reject) => {
    if (!socket) return reject(new Error("Chrome target unavailable"));
    socket.onopen = () => resolveSocket(); socket.onerror = () => reject(new Error("CDP failed"));
  });
  function call(method: string, params: Record<string, unknown> = {}) {
    if (!socket) throw new Error("Chrome target closed");
    const id = ++sequence; socket.send(JSON.stringify({ id, method, params }));
    return new Promise<any>((resolveCall, reject) => pending.set(id, { resolve: resolveCall, reject }));
  }
  async function evaluate<T>(expression: string): Promise<T> {
    const result = await call("Runtime.evaluate", { expression, awaitPromise: true, returnByValue: true, userGesture: true });
    if (result.exceptionDetails) throw new Error(result.exceptionDetails.text || "browser evaluation failed");
    return result.result?.value as T;
  }
  async function waitFor(expression: string, timeoutMs: number, label: string) {
    const deadline = Date.now() + timeoutMs; let last: unknown;
    while (Date.now() < deadline) {
      try { last = await evaluate(expression); if (last) return last; } catch (error) { last = String(error); }
      await sleep(100);
    }
    throw new Error(`${label} timed out; last=${JSON.stringify(last)} url=${await evaluate("location.href").catch(() => "?")}`);
  }
  async function navigate(url: string) {
    await call("Page.navigate", { url });
    await waitFor(`document.readyState === "complete" && location.href.startsWith(${JSON.stringify(base)})`, 20_000, `load ${url}`);
  }

  await call("Page.enable"); await call("Runtime.enable"); await call("Network.enable");
  await call("WebAuthn.enable");
  await call("Storage.clearDataForOrigin", { origin: base, storageTypes: "all" });
  await call("WebAuthn.addVirtualAuthenticator", { options: {
    protocol: "ctap2", transport: "internal", hasResidentKey: true,
    hasUserVerification: true, isUserVerified: true, automaticPresenceSimulation: true,
  }});

  await navigate(`${base}/setup/${setupToken}`);
  await waitFor(`location.pathname === "/" && document.querySelector("#setup-form")`, 10_000, "setup page");
  const setupHtml = await evaluate<string>("document.documentElement.outerHTML");
  if (!setupHtml.includes("Create the first account with a passkey.")) throw new Error("exact setup page missing");
  await evaluate(`(()=>{const input=document.querySelector('input[name="name"]');input.value="Identity Component E2E";document.querySelector('#setup-form').requestSubmit();return true})()`);
  await waitFor(`location.pathname === "/devices"`, 30_000, "component passkey setup");
  const status = await evaluate<any>(`fetch('/api/v1/status').then(r=>r.json())`);
  if (status.setupRequired !== false) throw new Error(`setup did not persist: ${JSON.stringify(status)}`);

  await evaluate(`fetch('/account/logout',{method:'POST',redirect:'manual'})`);
  await navigate(`${base}/`);
  await waitFor(`document.querySelector('.public-site') && document.body.textContent.includes('Remote Control')`, 10_000, "signed-out landing");

  await stopServer(); startServer(); await waitForHttp(`${base}/healthz`);
  await navigate(`${base}/login?next=%2Fdevices`);
  await waitFor(`document.querySelector("#login-form")`, 10_000, "login page after restart");
  await evaluate(`document.querySelector('#login-form').requestSubmit()`);
  await waitFor(`location.pathname === "/devices"`, 30_000, "component passkey login");
  await navigate(`${base}/login`);
  await waitFor(`location.pathname === "/devices"`, 10_000, "authenticated login redirect");
  console.log("component passkey setup, restart persistence, logout, and login passed");
} finally {
  await stopServer(); socket?.close();
  if (target) await fetch(`${cdp}/json/close/${target.id}`).catch(() => {});
  await rm(directory, { recursive: true, force: true });
}
