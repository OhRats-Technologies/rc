import type { User } from "../../../src/core";
import type { DeviceView } from "../../../src/devices";
import type { WorkspaceView } from "../../../src/workspaces";
import { htmlDocument } from "../document";
import { processLabel, processOriginLabel, processState, relative } from "../format";

type RemoteProcess = ReturnType<typeof import("../../../src/process-store").processJSON>;

function directControl(origin: string) { return ["browser", "cli", "api", "control"].includes(origin); }

export function processPage(user: User, workspaces: WorkspaceView[], device: DeviceView, process: RemoteProcess, sidebar: "open" | "closed") {
  const running = process.status === "starting" || process.status === "running";
  const controllable = device.role === "owner" || (device.role === "operator" && process.created_by === user.id);
  const interactive = directControl(process.origin) && Boolean(process.terminal) && running && controllable;
  const label = processLabel(process);
  return htmlDocument({ title: label, user, workspaces, path: `/devices/${device.id}/processes/${process.id}`, sidebar,
    scripts: interactive ? ["process-terminal"] : [], styles: interactive ? ["process-terminal"] : [], body:
    <div className="page process-page" data-process-page={process.id} data-device-id={device.id} data-process-status={process.status} data-process-live={running ? "true" : "false"} data-process-interactive={interactive ? "true" : "false"}>
      <header className="page-header process-header"><div><p className="eyebrow"><a href={`/devices/${device.id}`}>{device.name.toUpperCase()}</a> / PROCESS</p><h1 className="mono process-title">{label}</h1><p className="meta">{processOriginLabel(process.origin)} · STARTED BY {process.created_by_name || "UNKNOWN"} · {relative(process.created_at)}</p></div>
        <span id="process-state" className={`status${process.status === "running" ? " online" : ""}`}>{processState(process)}</span></header>
      <div id="terminal-toolbar" className="terminal-toolbar"><span className="terminal-label">{process.terminal ? "PTY" : "PROCESS"}/{process.id.slice(0, 8)}{interactive ? <> · <span id="control-transport">CONNECTING</span></> : null}</span>{interactive && <div id="terminal-actions" className="terminal-actions"><button className="text-button" data-signal="INT" type="button">CTRL-C</button><button className="text-button" data-signal="TERM" type="button">TERM</button><button className="text-button" data-signal="KILL" type="button">KILL</button></div>}</div>
      {interactive ? <><pre id="process-transcript" className="terminal-transcript">Terminal scrollback is retained in RC Node memory while this process is live.</pre><div id="terminal-host" className="terminal-host" hidden/><div className="mobile-terminal-keys" aria-label="Terminal keys"><button type="button" data-terminal-key="ESC">ESC</button><button type="button" data-terminal-key="CTRL">CTRL</button><button type="button" data-terminal-key="ALT">ALT</button><button type="button" data-terminal-key="TAB">TAB</button><button type="button" data-terminal-key="LEFT">←</button><button type="button" data-terminal-key="UP">↑</button><button type="button" data-terminal-key="DOWN">↓</button><button type="button" data-terminal-key="RIGHT">→</button></div></> : <pre className="terminal-transcript">{process.terminal ? "Terminal content is retained only in RC Node memory while the process is live." : "Process content is not retained by RC."}</pre>}
      <p id="process-message" className="meta process-message">{process.error || (running && !controllable ? `Live control belongs to ${process.created_by_name || "another operator"}.` : "")}</p>
    </div> });
}
