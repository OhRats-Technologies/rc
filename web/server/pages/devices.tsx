import type { DeviceView } from "../../../src/devices";
import type { WorkspaceView } from "../../../src/workspaces";
import { processLabel, processState, relative } from "../format";
import { htmlDocument } from "../document";
import type { User } from "../../../src/core";
import { SectionBadge } from "../components";

type RemoteProcess = ReturnType<typeof import("../../../src/process-store").processJSON>;

function platformIcon(platform: string) {
  const value = platform.toLowerCase();
  if (value === "darwin" || value === "macos") return "icon-platform-macos";
  if (value === "linux") return "icon-platform-linux";
  if (value === "windows" || value === "win32") return "icon-platform-windows";
  return "icon-devices";
}

function DeviceRows({ devices }: { devices: DeviceView[] }) {
  return <div className="data-list" id="device-list">
    {devices.length ? devices.map(device => <a className="data-row" key={device.id} href={`/devices/${device.id}`} data-device-row={device.id}>
      <div className="device-row-main"><span className={`ui-icon device-platform-icon ${platformIcon(device.platform)}`} aria-hidden="true"/><div><strong>{device.name}</strong><div className="meta">{device.workspace_name} · {device.platform.toUpperCase()}/{device.arch}{device.active_processes ? ` · ${device.active_processes} ACTIVE` : ""}</div></div></div>
      <span className={`status${device.online ? " online" : ""}`} data-device-status={device.id}>{device.online ? "ONLINE" : `SEEN ${relative(device.last_seen)}`}</span>
    </a>) : <p className="empty-state">No devices yet. Enroll one from a workspace.</p>}
  </div>;
}

function ProcessRows({ deviceId, processes }: { deviceId: string; processes: RemoteProcess[] }) {
  return <div className="data-list" id="process-list">
    {processes.length ? processes.map(process => <a className="data-row process-row" key={process.id} href={`/devices/${deviceId}/processes/${process.id}`} data-process-row={process.id}>
      <div><strong className="mono">{processLabel(process.command)}</strong><div className="meta">{process.cwd || "~"} · {process.created_by_name || "Unknown"} · {relative(process.created_at)}</div></div>
      <span className={`status${process.status === "running" ? " online" : ""}`} data-process-status={process.id}>{processState(process)}</span>
    </a>) : <p className="empty-state">No processes yet.</p>}
  </div>;
}

export function devicesPage(user: User, workspaces: WorkspaceView[], devices: DeviceView[], sidebar: "open" | "closed") {
  const canEnroll = workspaces.some(workspace => workspace.role === "owner");
  return htmlDocument({ title: "Devices", user, workspaces, path: "/devices", sidebar, scripts: ["live"], body:
    <div className="page" data-live-page="devices">
      <header className="page-header"><div><h1>Devices</h1></div>{canEnroll && <a className="header-icon-button" href="/devices/enroll" aria-label="Enroll device" title="Enroll device"><span className="ui-icon icon-plus" aria-hidden="true"/></a>}</header>
      <DeviceRows devices={devices}/>
    </div> });
}

export function devicePage(user: User, workspaces: WorkspaceView[], device: DeviceView, processes: RemoteProcess[], sidebar: "open" | "closed") {
  const supportsProcess = device.capabilities.includes("process");
  const canOperate = device.role === "owner" || device.role === "operator";
  return htmlDocument({ title: device.name, user, workspaces, path: `/devices/${device.id}`, sidebar, scripts: ["live", "device"], body:
    <div className="page" data-device-page={device.id} data-supports-process={supportsProcess ? "true" : "false"}>
      <section className="device-overview">
        <header className="page-header device-header">
          <div><p className="eyebrow">DEVICE</p><h1>{device.name}</h1><p className="meta">{device.workspace_name} · {device.platform.toUpperCase()}/{device.arch}</p></div>
          <div className="device-header-actions"><span id="device-status" className={`status${device.online ? " online" : ""}`}>{device.online ? "ONLINE" : `LAST SEEN ${relative(device.last_seen)}`}</span>
            {canOperate && <button id="open-terminal" className="device-terminal-button" type="button" aria-label="Open terminal" title="Open terminal" disabled={!device.online || !supportsProcess}><span className="ui-icon icon-terminal" aria-hidden="true"/></button>}
          </div>
        </header>
        <dl className="facts">
          <div><dt>HOST</dt><dd>{device.hostname}</dd></div><div><dt>NODE VERSION</dt><dd id="node-agent">{device.agent_version}</dd></div>
        </dl>
        <p id="process-error" className="error">{supportsProcess ? "" : <>This RC Node is too old for terminals. Run <code>ohrats-rc update</code> on the device.</>}</p>
      </section>
      <section className="content-section"><div className="section-heading"><div><SectionBadge index="01">Processes</SectionBadge><h2>History</h2></div></div>{device.role === "viewer" ? <p className="empty-state">Process history is available to operators and owners.</p> : <ProcessRows deviceId={device.id} processes={processes}/>}</section>
    </div> });
}
