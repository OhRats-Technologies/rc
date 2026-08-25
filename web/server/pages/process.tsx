import { Buffer } from "node:buffer";
import type { User } from "../../../src/core";
import type { DeviceView } from "../../../src/devices";
import type { WorkspaceView } from "../../../src/workspaces";
import { htmlDocument } from "../document";
import { LOGIN_SHELL_COMMAND, processLabel, processState, relative, terminalFallback } from "../format";

type RemoteProcess = ReturnType<typeof import("../../../src/process-store").processJSON>;

export function processPage(user: User, workspaces: WorkspaceView[], device: DeviceView, process: RemoteProcess, sidebar: "open" | "closed") {
  const running = process.status === "starting" || process.status === "running";
  const controllable = device.role === "owner" || (device.role === "operator" && process.created_by === user.id);
  const interactive = running && controllable;
  const label = processLabel(process.command);
  const source = Buffer.from(process.output || "", "utf8").toString("base64");
  return htmlDocument({ title: label, user, workspaces, path: `/devices/${device.id}/processes/${process.id}`, sidebar,
    scripts: ["process-terminal"], styles: ["process-terminal"], body:
    <div className="page process-page" data-process-page={process.id} data-device-id={device.id} data-process-revision={process.revision} data-process-status={process.status} data-process-live={running ? "true" : "false"} data-process-interactive={interactive ? "true" : "false"}>
      <header className="page-header process-header"><div><p className="eyebrow"><a href={`/devices/${device.id}`}>{device.name.toUpperCase()}</a> / PROCESS</p><h1 className="mono process-title">{label}</h1><p className="meta">{process.cwd || "~"} · STARTED BY {process.created_by_name || "UNKNOWN"} · {relative(process.created_at)}</p></div>
        <span id="process-state" className={`status${process.status === "running" ? " online" : ""}`}>{processState(process)}</span></header>
      <div id="terminal-toolbar" className="terminal-toolbar"><span className="terminal-label">PTY/{process.id.slice(0, 8)}</span>{interactive && <div id="terminal-actions" className="terminal-actions"><button className="text-button" data-signal="INT" type="button">CTRL-C</button><button className="text-button" data-signal="TERM" type="button">TERM</button><button className="text-button" data-signal="KILL" type="button">KILL</button></div>}</div>
      <pre id="process-transcript" className="terminal-transcript">{terminalFallback(process.output || "")}</pre><script id="process-terminal-source" type="application/octet-stream">{source}</script><div id="terminal-host" className="terminal-host" hidden/>{interactive && <div className="mobile-terminal-keys" aria-label="Terminal keys"><button type="button" data-terminal-key="ESC">ESC</button><button type="button" data-terminal-key="CTRL">CTRL</button><button type="button" data-terminal-key="ALT">ALT</button><button type="button" data-terminal-key="TAB">TAB</button><button type="button" data-terminal-key="LEFT">←</button><button type="button" data-terminal-key="UP">↑</button><button type="button" data-terminal-key="DOWN">↓</button><button type="button" data-terminal-key="RIGHT">→</button></div>}
      <p id="process-message" className="meta process-message">{process.error || (running && !controllable ? `Live control belongs to ${process.created_by_name || "another operator"}.` : "")}</p>
      {device.role === "owner" && process.command !== LOGIN_SHELL_COMMAND && <a className="text-action save-action" href={`/actions/new?workspace=${device.workspace_id}&process=${process.id}`}>SAVE AS ACTION →</a>}
    </div> });
}
