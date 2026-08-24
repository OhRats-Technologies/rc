import type { User } from "../../../src/core";
import type { DeviceView } from "../../../src/devices";
import type { WorkspaceView } from "../../../src/workspaces";
import { htmlDocument } from "../document";
import { processState, relative } from "../format";

type RemoteProcess = ReturnType<typeof import("../../../src/process-store").processJSON>;

export function processPage(user: User, workspaces: WorkspaceView[], device: DeviceView, process: RemoteProcess, sidebar: "open" | "closed") {
  return htmlDocument({ title: process.command, user, workspaces, path: `/devices/${device.id}/processes/${process.id}`, sidebar, scripts: ["process-terminal"], styles: ["process-terminal"], body:
    <div className="page process-page" data-process-page={process.id} data-device-id={device.id} data-process-revision={process.revision} data-process-status={process.status}>
      <header className="page-header process-header"><div><p className="eyebrow"><a href={`/devices/${device.id}`}>{device.name.toUpperCase()}</a> / PROCESS</p><h1 className="mono process-title">{process.command}</h1><p className="meta">{process.cwd || "~"} · STARTED {relative(process.created_at)}</p></div>
        <span id="process-state" className={`status${process.status === "running" ? " online" : ""}`}>{processState(process)}</span></header>
      <div id="terminal-toolbar" className="terminal-toolbar"><span className="terminal-label">PTY/{process.id.slice(0, 8)}</span><div className="terminal-actions"><button className="text-button" data-signal="INT" type="button">CTRL-C</button><button className="text-button" data-signal="TERM" type="button">TERM</button><button className="text-button" data-signal="KILL" type="button">KILL</button></div></div>
      <pre id="process-transcript" className="terminal-transcript">{process.output || ""}</pre><div id="terminal-host" className="terminal-host" hidden/>
      <p id="process-message" className="meta process-message">{process.error || ""}</p>
    </div> });
}
