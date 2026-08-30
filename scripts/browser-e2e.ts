import { chmod, copyFile, mkdtemp, mkdir, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { createServer } from "node:net";
import type { AddressInfo } from "node:net";

const root = resolve(import.meta.dir, "..");
const cdp = process.env.RC_CDP_URL || "http://127.0.0.1:9223";
const binary = process.env.RC_E2E_BINARY || join(root, "target/debug/rc");
const serverBinary = process.env.RC_E2E_SERVER || join(root, "target/debug/rc-server");
const kernelBinary = process.env.RC_E2E_KERNEL || join(root, "kernel/target/debug/rc-kernel");
const assets = process.env.RC_E2E_ASSETS || join(root, "dist/assets");
const keep = process.env.RC_E2E_KEEP === "1";

function sleep(ms: number) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

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

async function readStream(stream: ReadableStream<Uint8Array> | null) {
  return stream ? await new Response(stream).text() : "";
}

async function waitForHttp(url: string, timeoutMs = 20_000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    try {
      const response = await fetch(url);
      if (response.ok) return;
    } catch {}
    await sleep(100);
  }
  throw new Error(`HTTP endpoint did not become ready: ${url}`);
}

if (!await Bun.file(binary).exists() || !await Bun.file(serverBinary).exists() || !await Bun.file(kernelBinary).exists()) {
  throw new Error("build rc-server, rc, and rc-kernel before running browser E2E");
}
if (!await Bun.file(join(assets, "auth.js")).exists()) {
  throw new Error("run bun run build:client before running browser E2E");
}

const directory = await mkdtemp(join(tmpdir(), "rc-browser-e2e-"));
const data = join(directory, "data"), nodeState = join(directory, "node"), components = join(directory, "components");
await mkdir(data); await mkdir(nodeState); await mkdir(components);
const loginShell = join(directory, "login-shell");
await writeFile(loginShell, "#!/bin/sh\nprintf 'RC_BROWSER_E2E_OK\\n'\n", { mode: 0o700 });
await chmod(loginShell, 0o700);
for (const name of [
  "diagnostics-store", "process-policy", "shell",
  "execution-runtime", "scheduler", "transport-webrtc",
]) {
  const source = join(root, "dist/components", `${name}.wasm`);
  if (!await Bun.file(source).exists()) throw new Error(`build ${name} before running browser E2E`);
  await copyFile(source, join(components, `${name}.wasm`));
}
const port = Number(process.env.RC_E2E_PORT || await freePort());
const sshPort = await freePort(), sshInternalPort = await freePort();
const localBase = `http://localhost:${port}`;
const base = (process.env.RC_E2E_PUBLIC_URL || localBase).replace(/\/$/, "");
const nodeBase = (process.env.RC_E2E_NODE_URL || localBase).replace(/\/$/, "");
const setupToken = `e2e-${crypto.randomUUID()}`;
const server = Bun.spawn([serverBinary], {
  env: {
    ...process.env,
    PORT: String(port),
    DATA_DIR: data,
    RC_DB_PATH: join(data, "rc.sqlite3"),
    STATIC_DIR: assets,
    PUBLIC_URL: base,
    RC_SETUP_TOKEN: setupToken,
    RC_SSH_DAEMON_PORT: String(sshPort),
    RC_SSH_INTERNAL_PORT: String(sshInternalPort),
    RUST_LOG: "rc_server=info",
  },
  stdout: "inherit",
  stderr: "inherit",
});
let node: ReturnType<typeof Bun.spawn> | null = null;
let target: { id: string; webSocketDebuggerUrl: string } | null = null;
let socket: WebSocket | null = null;

try {
  await waitForHttp(`${localBase}/healthz`);
  const createdTarget = await fetch(`${cdp}/json/new?${encodeURIComponent("about:blank")}`, { method: "PUT" })
    .then(async response => {
      if (!response.ok) throw new Error(`create Chrome target: ${response.status} ${await response.text()}`);
      return await response.json() as { id: string; webSocketDebuggerUrl: string };
    });
  target = createdTarget;
  socket = new WebSocket(createdTarget.webSocketDebuggerUrl);
  let sequence = 0;
  const pending = new Map<number, { resolve(value: any): void; reject(error: Error): void }>();
  socket.onmessage = event => {
    const message = JSON.parse(String(event.data));
    const waiter = pending.get(message.id);
    if (!waiter) return;
    pending.delete(message.id);
    if (message.error) waiter.reject(new Error(`${message.error.message}: ${JSON.stringify(message.error.data || {})}`));
    else waiter.resolve(message.result || {});
  };
  await new Promise<void>((resolveSocket, reject) => {
    if (!socket) return reject(new Error("Chrome target unavailable"));
    socket.onopen = () => resolveSocket();
    socket.onerror = () => reject(new Error("Chrome DevTools WebSocket failed"));
  });
  function call(method: string, params: Record<string, unknown> = {}) {
    if (!socket) throw new Error("Chrome DevTools socket closed");
    const id = ++sequence;
    socket.send(JSON.stringify({ id, method, params }));
    return new Promise<any>((resolveCall, reject) => pending.set(id, { resolve: resolveCall, reject }));
  }
  async function evaluate<T>(expression: string): Promise<T> {
    const result = await call("Runtime.evaluate", {
      expression, awaitPromise: true, returnByValue: true, userGesture: true,
    });
    if (result.exceptionDetails) throw new Error(result.exceptionDetails.text || "browser evaluation failed");
    return result.result?.value as T;
  }
  async function waitFor(expression: string, timeoutMs: number, label: string) {
    const deadline = Date.now() + timeoutMs;
    let last: unknown;
    while (Date.now() < deadline) {
      try { last = await evaluate(expression); if (last) return last; }
      catch (error) { last = String(error); }
      await sleep(100);
    }
    const url = await evaluate<string>("location.href").catch(() => "unavailable");
    throw new Error(`${label} timed out; last=${JSON.stringify(last)} url=${url}`);
  }
  async function navigate(url: string) {
    await call("Page.navigate", { url });
    await waitFor(`document.readyState === "complete" && location.href.startsWith(${JSON.stringify(base)})`, 20_000, `load ${url}`);
  }
  async function browserFetch<T>(path: string, options = "{}") {
    return await evaluate<T>(`(async()=>{const response=await fetch(${JSON.stringify(path)},${options});const text=await response.text();let body;try{body=JSON.parse(text)}catch{body=text}if(!response.ok)throw new Error(response.status+" "+JSON.stringify(body));return body})()`);
  }

  await call("Page.enable"); await call("Runtime.enable"); await call("Network.enable");
  await call("Page.addScriptToEvaluateOnNewDocument", { source: `
    (() => {
      const original = SubtleCrypto.prototype.decrypt;
      const originalFetch = window.fetch.bind(window);
      Object.defineProperty(window, "__rcE2EOutput", { value: "", writable: true, configurable: true });
      Object.defineProperty(window, "__rcE2ERtc", { value: { offers: [], answers: [] }, configurable: true });
      const summarizeCandidates = sdp => String(sdp || "").split(String.fromCharCode(10)).filter(line => line.startsWith("a=candidate:"))
        .map(line => { const fields = line.slice(12).trim().split(" ").filter(Boolean), typeAt = fields.indexOf("typ"), address = fields[4] || "";
          return { protocol: fields[2] || "", address: address.endsWith(".local") ? "mdns" : address.includes(":") ? "ipv6" : "ipv4", type: typeAt >= 0 ? fields[typeAt + 1] : "" }; });
      window.fetch = async (...args) => {
        const input = args[0], options = args[1] || {};
        const requestUrl = new URL(typeof input === "string" ? input : input.url, location.href);
        const signaling = requestUrl.pathname.startsWith("/api/v1/control/") && requestUrl.pathname.endsWith("/webrtc");
        if (signaling && typeof options.body === "string") {
          try { window.__rcE2ERtc.offers.push(summarizeCandidates(JSON.parse(options.body).sdp)); } catch {}
        }
        const response = await originalFetch(...args);
        if (signaling && response.ok) {
          try { window.__rcE2ERtc.answers.push(summarizeCandidates((await response.clone().json()).sdp)); } catch {}
        }
        if (requestUrl.pathname !== "/api/v1/control/open" || !response.ok) return response;
        const body = await response.clone().json();
        body.iceServers = [...(body.iceServers || []), {
          urls: "turn:127.0.0.1:9?transport=udp", username: "e2e", credential: "unreachable",
        }];
        const headers = new Headers(response.headers); headers.delete("content-length");
        return new Response(JSON.stringify(body), { status: response.status, statusText: response.statusText, headers });
      };
      SubtleCrypto.prototype.decrypt = async function(...args) {
        const result = await original.apply(this, args);
        try {
          const message = JSON.parse(new TextDecoder().decode(result));
          if ((message.type === "process.stdout" || message.type === "process.stderr") && message.data) {
            const base64 = String(message.data).replace(/-/g, "+").replace(/_/g, "/");
            const padded = base64 + "=".repeat((4 - base64.length % 4) % 4);
            const bytes = Uint8Array.from(atob(padded), value => value.charCodeAt(0));
            window.__rcE2EOutput += new TextDecoder().decode(bytes);
          }
        } catch {}
        return result;
      };
    })();
  ` });
  await call("WebAuthn.enable");
  await call("Storage.clearDataForOrigin", { origin: base, storageTypes: "all" });
  await call("WebAuthn.addVirtualAuthenticator", { options: {
    protocol: "ctap2", transport: "internal", hasResidentKey: true,
    hasUserVerification: true, isUserVerified: true, automaticPresenceSimulation: true,
  }});

  await navigate(`${base}/setup/${setupToken}`);
  await waitFor(`document.querySelector("#setup-form") !== null`, 10_000, "setup form");
  await evaluate(`(()=>{const input=document.querySelector('input[name="name"]');input.value="RC Browser E2E";document.querySelector('#setup-form').requestSubmit();return true})()`);
  await waitFor(`location.pathname === "/devices"`, 30_000, "passkey setup");
  const me = await browserFetch<{ workspaces: Array<{ id: string; role: string }> }>("/api/v1/me");
  const workspace = me.workspaces.find(value => value.role === "owner");
  if (!workspace) throw new Error("setup produced no owned workspace");
  const enrollment = await browserFetch<{ token: string }>(`/api/v1/workspaces/${workspace.id}/enrollments`, `{
    method:"POST",credentials:"same-origin",headers:{"content-type":"application/json"},body:"{}"
  }`);
  const enroll = Bun.spawn([binary, "enroll", enrollment.token, "--url", nodeBase, "--name", "Browser E2E Node", "--state-dir", nodeState], {
    stdout: "pipe", stderr: "pipe",
  });
  const [code, output, error] = await Promise.all([enroll.exited, readStream(enroll.stdout), readStream(enroll.stderr)]);
  if (code !== 0) throw new Error(`node enrollment failed (${code}): ${output}\n${error}`);
  node = Bun.spawn([binary, "run", "--state-dir", nodeState], {
    env: { ...Bun.env, RC_KERNEL: kernelBinary, RC_COMPONENT_DIR: components, RC_SHELL: loginShell },
    stdout: "inherit", stderr: "inherit",
  });
  const device = await waitFor(`(async()=>{const r=await fetch('/api/v1/devices');if(!r.ok)return false;const j=await r.json();return j.devices.find(d=>d.name==='Browser E2E Node'&&d.online)||false})()`, 30_000, "Node online") as { id: string };
  await waitFor(`(async()=>{const r=await fetch(${JSON.stringify(`/api/v1/workspaces/${workspace.id}/authority`)});if(!r.ok)return false;const j=await r.json();return j.devices>0&&j.synced===j.devices})()`, 30_000, "RC Lock bootstrap");
  const process = await browserFetch<{ processId: string }>(`/api/v1/devices/${device.id}/processes`, `{
    method:"POST",credentials:"same-origin",headers:{"content-type":"application/json"},body:JSON.stringify({terminal:true})
  }`);
  await evaluate(`(()=>{sessionStorage.setItem(${JSON.stringify(`rc_process_start_${process.processId}`)},JSON.stringify({mode:{kind:"systemLoginShell"},terminal:{cols:80,rows:24,term:"xterm-256color"}}));location.href=${JSON.stringify(`/devices/${device.id}/processes/${process.processId}`)};return true})()`);
  await waitFor(`location.pathname.endsWith(${JSON.stringify(`/processes/${process.processId}`)}) && document.readyState === "complete"`, 20_000, "process page");
  const terminalFonts = await evaluate<string[]>(`[getComputedStyle(document.querySelector('.terminal-host')).fontFamily,getComputedStyle(document.querySelector('.terminal-host .xterm')).fontFamily]`);
  if (terminalFonts.some(font => !font.includes("MesloLGS Nerd Font Mono"))) throw new Error(`terminal font did not use Nerd Font: ${terminalFonts.join(" / ")}`);
  try {
    await waitFor(`document.querySelector('#control-transport')?.textContent?.includes('WEBRTC') && !document.querySelector('#control-transport')?.textContent?.includes('FAILED')`, 35_000, "WebRTC control transport");
  } catch (failure) {
    const diagnostics = await evaluate(`({ transport: document.querySelector('#control-transport')?.textContent,
      title: document.querySelector('#control-transport')?.title, message: document.querySelector('#process-message')?.textContent,
      clientError: document.querySelector('#process-client-error')?.textContent,
      rtc: window.__rcE2ERtc, scripts: Array.from(document.scripts).map(script => script.src),
      resources: performance.getEntriesByType('resource').filter(entry => entry.name.includes('/api/v1/control/') || entry.name.includes('/assets/process-terminal'))
        .map(entry => ({ name: entry.name, duration: entry.duration, size: entry.transferSize })) })`).catch(() => null);
    throw new Error(`${String(failure)} diagnostics=${JSON.stringify(diagnostics)}`);
  }
  await waitFor(`String(window.__rcE2EOutput || '').includes('RC_BROWSER_E2E_OK')`, 35_000, "terminal output");
  await waitFor(`(async()=>{const r=await fetch(${JSON.stringify(`/api/v1/processes/${process.processId}`)});if(!r.ok)return false;const j=await r.json();return j.process?.status==='exited'})()`, 35_000, "process exit");
  const terminalText = await evaluate<string>(`window.__rcE2EOutput || ""`);
  if (!terminalText.includes("RC_BROWSER_E2E_OK")) throw new Error("terminal output did not traverse encrypted WebRTC control");
  const transport = await evaluate<string>(`document.querySelector('#control-transport')?.textContent || ""`);
  if (transport.includes("TURN")) throw new Error(`direct browser route unexpectedly used ${transport}`);
  console.log(`browser command passed over ${transport}`);

  await evaluate(`(()=>{document.querySelector('form[action="/account/logout"]').requestSubmit();return true})()`);
  await waitFor(`location.pathname === "/" && document.querySelector('.public-site') !== null`, 15_000, "logout landing");
  const landing = await evaluate<string>("document.documentElement.outerHTML");
  if (!landing.includes('Remote Control<br><span class="hero-muted">for your machines.</span>')) throw new Error("restored landing missing after logout");
  await navigate(`${base}/devices`);
  await waitFor(`location.pathname === "/login"`, 15_000, "logged-out redirect");
  await navigate(`${base}/docs`);
  const docs = await evaluate<string>("document.documentElement.outerHTML");
  for (const needle of ["docs-sidebar", "docs-mobile-catalog", "docs-toc", "Quickstart", "Security model", "Authentication"]) {
    if (!docs.includes(needle)) throw new Error(`restored docs missing ${needle}`);
  }
  console.log("passkey setup, Node enrollment, encrypted browser control, logout, landing, and docs passed");
} finally {
  if (node) { node.kill("SIGTERM"); await Promise.race([node.exited, sleep(3000)]).catch(() => {}); }
  server.kill("SIGTERM"); await Promise.race([server.exited, sleep(3000)]).catch(() => {});
  socket?.close();
  if (target) await fetch(`${cdp}/json/close/${target.id}`).catch(() => {});
  if (!keep) await rm(directory, { recursive: true, force: true });
  else console.log(`kept E2E state at ${directory}`);
}
