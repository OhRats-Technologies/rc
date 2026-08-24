import "@xterm/xterm/css/xterm.css";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { api, escapeHTML, qs, relative } from "../api";
import { onRelayEvent, relayRequest, relaySend } from "../events";
import type { Device, RemoteProcess } from "../types";

function terminalTheme() {
  const style = getComputedStyle(document.documentElement), value = (name: string) => style.getPropertyValue(name).trim();
  return { background: value("--or-bg"), foreground: value("--or-text"), cursor: value("--or-text"), selectionBackground: value("--or-surface-hover") };
}

function stateText(process: RemoteProcess) {
  if (process.status === "starting") return "STARTING";
  if (process.status === "running") return "RUNNING";
  if (process.status === "lost") return "LOST";
  return process.signal || `EXIT ${process.exit_code ?? "?"}`;
}

export async function renderProcess(deviceId: string, processId: string) {
  const [{ device }, { process }] = await Promise.all([
    api<{ device: Device }>(`/api/v1/devices/${deviceId}`), api<{ process: RemoteProcess }>(`/api/v1/processes/${processId}`),
  ]);
  if (process.device_id !== deviceId) throw new Error("Process not found");
  qs<HTMLElement>("#page").innerHTML = `<div class="page process-page">
    <header class="page-header process-header">
      <div><p class="eyebrow"><a href="/devices/${deviceId}">${escapeHTML(device.name.toUpperCase())}</a> / PROCESS</p><h1 class="mono process-title">${escapeHTML(process.command)}</h1><p class="meta">${escapeHTML(process.cwd || "~")} · STARTED ${relative(process.created_at)}</p></div>
      <span id="process-state" class="status ${process.status === "running" ? "online" : ""}">${escapeHTML(stateText(process))}</span>
    </header>
    <div class="terminal-toolbar"><span class="terminal-label">PTY/${process.id.slice(0, 8)}</span><div class="terminal-actions"><button class="text-button" data-signal="INT" type="button">CTRL-C</button><button class="text-button" data-signal="TERM" type="button">TERM</button><button class="text-button" data-signal="KILL" type="button">KILL</button></div></div>
    <div id="terminal-host" class="terminal-host"></div>
    <p id="process-message" class="meta process-message"></p>
  </div>`;

  const host = qs<HTMLElement>("#terminal-host");
  const terminal = new Terminal({ cursorBlink: process.status === "running", scrollback: 10_000, theme: terminalTheme() });
  const fit = new FitAddon(); terminal.loadAddon(fit); terminal.open(host); terminal.write(process.output || "");
  let current = process, lastRevision = Number(process.revision || 0), fitFrame = 0;
  const fitTerminal = () => {
    cancelAnimationFrame(fitFrame);
    fitFrame = requestAnimationFrame(() => { try { fit.fit(); } catch {} });
  };
  fitTerminal();
  const observer = new ResizeObserver(fitTerminal); observer.observe(host);
  const dataDisposable = terminal.onData(data => { if (current.status === "running") relaySend("process.input", { processId, data }); });
  const resizeDisposable = terminal.onResize(size => { if (current.status === "running") relaySend("process.resize", { processId, cols: size.cols, rows: size.rows }); });
  terminal.focus();

  async function resync() {
    const snapshot = (await api<{ process: RemoteProcess }>(`/api/v1/processes/${processId}`)).process;
    current = snapshot; lastRevision = Number(snapshot.revision || 0);
    terminal.reset(); terminal.write(snapshot.output || ""); terminal.options.cursorBlink = snapshot.status === "running";
    const state = qs<HTMLElement>("#process-state"); state.textContent = stateText(snapshot); state.classList.toggle("online", snapshot.status === "running");
    qs<HTMLElement>("#process-message").textContent = snapshot.error || "";
    fitTerminal();
  }

  document.querySelectorAll<HTMLButtonElement>("[data-signal]").forEach(button => button.addEventListener("click", async () => {
    try { await relayRequest("process.signal", { processId, signal: button.dataset.signal }); }
    catch (error) { qs<HTMLElement>("#process-message").textContent = error instanceof Error ? error.message : String(error); }
  }));

  const unsubscribe = onRelayEvent(event => {
    if (event.kind === "relay.connected") { void resync(); return; }
    if (event.processId !== processId) return;
    if (event.kind === "process.output" && event.detail?.chunk) {
      const revision = Number(event.detail.revision || 0);
      if (!lastRevision || revision === lastRevision + 1) {
        terminal.write(String(event.detail.chunk)); lastRevision = revision;
      } else void resync();
      return;
    }
    if (["process.started", "process.exited", "process.lost"].includes(event.kind)) void resync();
  });

  return () => {
    unsubscribe(); observer.disconnect(); cancelAnimationFrame(fitFrame); dataDisposable.dispose(); resizeDisposable.dispose(); terminal.dispose();
  };
}
