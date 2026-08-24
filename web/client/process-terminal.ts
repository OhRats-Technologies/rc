import "@xterm/xterm/css/xterm.css";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { api, qs } from "./http";
import { fire, onEvent, request } from "./socket";
import type { RemoteProcess } from "../types";

const page = qs<HTMLElement>("[data-process-page]"), processId = page.dataset.processPage || "";
const transcript = qs<HTMLElement>("#process-transcript"), host = qs<HTMLElement>("#terminal-host");
const style = getComputedStyle(document.documentElement), color = (name: string) => style.getPropertyValue(name).trim();
const terminal = new Terminal({ cursorBlink: page.dataset.processStatus === "running", scrollback: 10_000,
  theme: { background: color("--or-bg"), foreground: color("--or-text"), cursor: color("--or-text"), selectionBackground: color("--or-surface-hover") } });
const fit = new FitAddon(); terminal.loadAddon(fit); host.hidden = false; transcript.hidden = true; terminal.open(host); terminal.write(transcript.textContent || "");
let status = page.dataset.processStatus || "", revision = Number(page.dataset.processRevision || 0), frame = 0;
const fitTerminal = () => { cancelAnimationFrame(frame); frame = requestAnimationFrame(() => { try { fit.fit(); } catch {} }); };
fitTerminal(); const observer = new ResizeObserver(fitTerminal); observer.observe(host);

terminal.onData(data => { if (status === "running") fire({ type: "process.input", processId, data }); });
terminal.onResize(size => { if (status === "running") fire({ type: "process.resize", processId, cols: size.cols, rows: size.rows }); });
terminal.focus();

function stateText(process: RemoteProcess) {
  if (process.status === "starting") return "STARTING"; if (process.status === "running") return "RUNNING";
  if (process.status === "lost") return "LOST"; return process.signal || `EXIT ${process.exit_code ?? "?"}`;
}
async function resync() {
  const { process } = await api<{ process: RemoteProcess }>(`/api/v1/processes/${processId}`);
  status = process.status; revision = Number(process.revision || 0); terminal.reset(); terminal.write(process.output || "");
  terminal.options.cursorBlink = status === "running"; const state = qs<HTMLElement>("#process-state");
  state.textContent = stateText(process); state.classList.toggle("online", status === "running");
  const actions = document.querySelector<HTMLElement>("#terminal-actions");
  if (actions) actions.hidden = !["starting", "running"].includes(status);
  qs<HTMLElement>("#process-message").textContent = process.error || ""; fitTerminal();
}

document.querySelectorAll<HTMLButtonElement>("[data-signal]").forEach(button => button.addEventListener("click", async () => {
  try { await request({ type: "process.signal", processId, signal: button.dataset.signal as "INT" | "TERM" | "KILL" }); }
  catch (error) { qs<HTMLElement>("#process-message").textContent = error instanceof Error ? error.message : String(error); }
}));

onEvent(event => {
  if (event.kind === "relay.connected") { void resync(); return; }
  if (event.processId !== processId) return;
  if (event.kind === "process.output" && event.detail?.chunk) {
    const next = Number(event.detail.revision || 0);
    if (!revision || next === revision + 1) { terminal.write(String(event.detail.chunk)); revision = next; }
    else void resync();
    return;
  }
  if (["process.started", "process.exited", "process.lost"].includes(event.kind)) void resync();
});

addEventListener("pagehide", () => { observer.disconnect(); cancelAnimationFrame(frame); terminal.dispose(); }, { once: true });
