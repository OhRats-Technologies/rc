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
  const canManage = device.role === "owner";
  return htmlDocument({ title: device.name, user, workspaces, path: `/devices/${device.id}`, sidebar, scripts: ["live", "device"], body:
    <div className="page" data-device-page={device.id} data-supports-process={supportsProcess ? "true" : "false"}>
      <section className="device-overview">
        <header className="page-header device-header">
          <div><p className="eyebrow">DEVICE</p><div className="page-title-row device-title-row"><span className={`ui-icon device-platform-icon device-title-platform ${platformIcon(device.platform)}`} aria-hidden="true"/><h1 data-device-title-view>{device.name}</h1>{canManage && <form method="post" action={`/devices/${device.id}/rename`} hidden data-device-title-form><input className="device-title-input" name="name" defaultValue={device.name} aria-label="Device name" required maxLength={120}/><input type="hidden" name="next" value={`/devices/${device.id}`}/></form>}{canManage && <button className="header-icon-button" type="button" data-device-title-rename aria-label="Rename device" title="Rename device"><span className="ui-icon icon-pencil" aria-hidden="true"/></button>}</div><p className="error device-title-error" data-device-title-error/><p className="meta">{device.workspace_name} · {device.platform.toUpperCase()}/{device.arch}</p></div>
          <div className="device-header-actions"><span id="device-status" className={`status${device.online ? " online" : ""}`}>{device.online ? "ONLINE" : `LAST SEEN ${relative(device.last_seen)}`}</span>
            {canOperate && <button id="open-terminal" className="device-terminal-button" type="button" aria-label="Open terminal" title="Open terminal" disabled={!device.online || !supportsProcess}><span className="ui-icon icon-terminal" aria-hidden="true"/></button>}{canManage && <button className="header-icon-button danger-icon-button" type="button" aria-label={`Delete ${device.name}`} title="Delete device" data-delete-kind="device" data-delete-name={device.name} data-delete-endpoint={`/api/v1/devices/${device.id}`} data-delete-redirect="/devices"><span className="ui-icon icon-trash" aria-hidden="true"/></button>}
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
