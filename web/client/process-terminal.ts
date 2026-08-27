import "@xterm/xterm/css/xterm.css";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebglAddon } from "@xterm/addon-webgl";
import { api, qs } from "./http";
import { onEvent } from "./socket";
import { openControlSession, type ControlSession } from "./control-session";
import type { ControlTransportStatus } from "./control-webrtc";
import { b64urlToBytes, bytesToB64url } from "./control-client";
import type { RemoteProcess } from "../types";

const page = qs<HTMLElement>("[data-process-page]"), processId = page.dataset.processPage || "";
const transcript = qs<HTMLElement>("#process-transcript"), source = qs<HTMLElement>("#process-terminal-source"), host = qs<HTMLElement>("#terminal-host");
const live = page.dataset.processLive === "true", interactive = page.dataset.processInteractive === "true", encrypted = page.dataset.processEncrypted === "true";
const encoded = source.textContent || "", bytes = Uint8Array.from(atob(encoded), value => value.charCodeAt(0)), initialOutput = new TextDecoder().decode(bytes);
const style = getComputedStyle(document.documentElement), color = (name: string) => style.getPropertyValue(name).trim();
const terminal = new Terminal({ cursorBlink: interactive && page.dataset.processStatus === "running", disableStdin: !interactive, scrollback: 10_000,
  theme: { background: color("--or-bg"), foreground: color("--or-text"), cursor: color("--or-text"), selectionBackground: color("--or-surface-hover") } });
const fit = new FitAddon(); terminal.loadAddon(fit); host.hidden = false; transcript.hidden = true; terminal.open(host);
let webgl: WebglAddon | null = null;
try {
  webgl = new WebglAddon();
  terminal.loadAddon(webgl);
  webgl.onContextLoss(() => {
    webgl?.dispose();
    webgl = null;
  });
} catch {
  webgl?.dispose();
  webgl = null;
}
terminal.write(initialOutput);
let status = page.dataset.processStatus || "", revision = Number(page.dataset.processRevision || 0), frame = 0;
let control: ControlSession | null = null, controlGeneration = 0;
let controlConnecting = false;
let ctrlNext = false, altNext = false;
let transportMessage = "";
const fitTerminal = () => { cancelAnimationFrame(frame); frame = requestAnimationFrame(() => { try { fit.fit(); } catch {} }); };
fitTerminal(); const observer = new ResizeObserver(fitTerminal); observer.observe(host);

function sendInput(data: string) {
  if (status === "running" && data) void control?.send({
    type: "process.stdin", id: processId, data: bytesToB64url(new TextEncoder().encode(data)),
  });
}

function transportText(status: ControlTransportStatus) {
  if (status.transport === "webrtc" && status.phase === "connecting") return "WEBRTC…";
  if (status.transport === "webrtc") {
    const pair = status.selected, route = pair?.localType && pair?.remoteType ? `${pair.localType}↔${pair.remoteType}` : "DIRECT";
    return `WEBRTC · ${route}${pair?.protocol ? `/${pair.protocol}` : ""}`.toUpperCase();
  }
  return "RELAY · WSS";
}

function showTransport(status: ControlTransportStatus) {
  const element = document.querySelector<HTMLElement>("#control-transport"); if (!element) return;
  element.textContent = transportText(status);
  element.title = status.reason || (status.transport === "webrtc" ? "Direct WebRTC DataChannel" : "Encrypted WebSocket relay");
  if (status.transport === "relay" && status.reason) {
    transportMessage = `WebRTC fallback: ${status.reason}`;
  } else if (status.transport === "webrtc" && status.phase === "connected") transportMessage = "";
  const message = qs<HTMLElement>("#process-message");
  if (!message.textContent?.trim() || transportMessage) message.textContent = transportMessage;
}
if (interactive) terminal.onData(data => {
  if (ctrlNext && data.length === 1) { const code = data.toUpperCase().charCodeAt(0); if (code >= 64 && code <= 95) data = String.fromCharCode(code - 64); ctrlNext = false; }
  if (altNext) { data = `\x1b${data}`; altNext = false; }
  document.querySelectorAll("[data-terminal-key='CTRL'],[data-terminal-key='ALT']").forEach(button => button.classList.remove("active"));
  sendInput(data);
});
if (interactive) terminal.onResize(size => { if (status === "running") void control?.send({ type: "process.resize", id: processId, cols: size.cols, rows: size.rows }); });
if (interactive) terminal.focus();

function stateText(process: RemoteProcess) {
  if (process.status === "starting") return "STARTING"; if (process.status === "running") return "RUNNING";
  if (process.status === "lost") return "LOST"; return process.signal || `EXIT ${process.exit_code ?? "?"}`;
}
async function resync() {
  const { process } = await api<{ process: RemoteProcess }>(`/api/v1/processes/${processId}`);
  status = process.status; revision = Number(process.revision || 0);
  if (!encrypted) { terminal.reset(); terminal.write(process.output || ""); }
  terminal.options.cursorBlink = interactive && status === "running"; const state = qs<HTMLElement>("#process-state");
  state.textContent = stateText(process); state.classList.toggle("online", status === "running");
  const actions = document.querySelector<HTMLElement>("#terminal-actions");
  if (actions) actions.hidden = !["starting", "running"].includes(status);
  qs<HTMLElement>("#process-message").textContent = process.error || transportMessage; fitTerminal();
}

function applyProcessEvent(event: { kind: string; detail?: Record<string, unknown> }) {
  const state = qs<HTMLElement>("#process-state"), actions = document.querySelector<HTMLElement>("#terminal-actions");
  if (event.kind === "process.started") {
    status = "running"; state.textContent = "RUNNING"; state.classList.add("online");
    terminal.options.cursorBlink = interactive; if (actions) actions.hidden = false; return;
  }
  if (event.kind === "process.exited") {
    status = "exited"; state.classList.remove("online"); terminal.options.cursorBlink = false;
    const signal = String(event.detail?.signal || ""), exitCode = event.detail?.exitCode;
    state.textContent = signal || `EXIT ${exitCode ?? "?"}`; if (actions) actions.hidden = true; return;
  }
  if (event.kind === "process.lost") {
    status = "lost"; state.textContent = "LOST"; state.classList.remove("online"); terminal.options.cursorBlink = false;
    if (actions) actions.hidden = true;
    qs<HTMLElement>("#process-message").textContent = String(event.detail?.error || "Process lost");
  }
}

if (interactive) document.querySelectorAll<HTMLButtonElement>("[data-signal]").forEach(button => button.addEventListener("click", async () => {
  try {
    if (!control) throw new Error("Control session reconnecting");
    await control.send({ type: "process.signal", id: processId, signal: button.dataset.signal as "INT" | "TERM" | "KILL" });
  }
  catch (error) { qs<HTMLElement>("#process-message").textContent = error instanceof Error ? error.message : String(error); }
}));

const keyValues: Record<string, string> = { ESC: "\x1b", TAB: "\t", LEFT: "\x1b[D", UP: "\x1b[A", DOWN: "\x1b[B", RIGHT: "\x1b[C" };
if (interactive) document.querySelectorAll<HTMLButtonElement>("[data-terminal-key]").forEach(button => button.addEventListener("click", () => {
  const key = button.dataset.terminalKey || "";
  if (key === "CTRL") { ctrlNext = !ctrlNext; button.classList.toggle("active", ctrlNext); terminal.focus(); return; }
  if (key === "ALT") { altNext = !altNext; button.classList.toggle("active", altNext); terminal.focus(); return; }
  sendInput(keyValues[key] || ""); terminal.focus();
}));

async function connectEncrypted() {
  if (!live || !interactive || !encrypted) return;
  if (controlConnecting) return;
  controlConnecting = true;
  const generation = ++controlGeneration; control?.close(); control = null;
  try {
    const next = await openControlSession(page.dataset.deviceId || "", showTransport);
    if (generation !== controlGeneration) { next.close(); return; }
    control = next;
    next.onMessage(message => {
      if (String(message.id || "") !== processId && !["control.result"].includes(message.type)) return;
      if (message.type === "process.stdout" && message.data) terminal.write(b64urlToBytes(String(message.data)));
      if (message.type === "process.stderr" && message.data) terminal.write(b64urlToBytes(String(message.data)));
      if (message.type === "process.started") { status = "running"; qs<HTMLElement>("#process-state").textContent = "RUNNING"; }
      if (message.type === "process.exit") {
        status = "exited"; const signal = String(message.signal || ""), exitCode = Number(message.exitCode ?? -1);
        qs<HTMLElement>("#process-state").textContent = signal || `EXIT ${exitCode}`;
        const actions = document.querySelector<HTMLElement>("#terminal-actions"); if (actions) actions.hidden = true;
      }
    });
    const key = `rc_process_start_${processId}`, raw = sessionStorage.getItem(key);
    if (raw) {
      sessionStorage.removeItem(key); const start = JSON.parse(raw) as {
        command: string; cwd: string; terminal: { cols: number; rows: number; term?: string };
      };
      await next.send({ type: "process.start", id: processId, command: start.command, cwd: start.cwd, terminal: start.terminal });
    } else await next.send({ type: "process.attach", id: processId });
  } catch (error) { qs<HTMLElement>("#process-message").textContent = error instanceof Error ? error.message : String(error); }
  finally { if (generation === controlGeneration) controlConnecting = false; }
}

if (live) onEvent(event => {
  if (event.kind === "rc.connected") { if (encrypted) void connectEncrypted(); else void resync(); return; }
  if (event.processId !== processId) return;
  if (["process.started", "process.exited", "process.lost"].includes(event.kind)) { applyProcessEvent(event); return; }
  if (encrypted) return;
  if (event.kind === "process.output" && event.detail?.chunk) {
    const next = Number(event.detail.revision || 0);
    if (!revision || next === revision + 1) { terminal.write(String(event.detail.chunk)); revision = next; }
    else void resync();
    return;
  }
});

if (encrypted) void connectEncrypted();

addEventListener("pagehide", () => {
  controlGeneration++; control?.close(); observer.disconnect(); cancelAnimationFrame(frame); webgl?.dispose(); terminal.dispose();
}, { once: true });
