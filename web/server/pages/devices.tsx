import type { DeviceView } from "../../../src/devices";
import type { WorkspaceView } from "../../../src/workspaces";
import { processLabel, processState, relative } from "../format";
import { htmlDocument } from "../document";
import type { User } from "../../../src/core";
import { SectionBadge } from "../components";

type RemoteProcess = ReturnType<typeof import("../../../src/process-store").processJSON>;

function DeviceRows({ devices }: { devices: DeviceView[] }) {
  return <div className="data-list" id="device-list">
    {devices.length ? devices.map(device => <a className="data-row" key={device.id} href={`/devices/${device.id}`} data-device-row={device.id}>
      <div><strong>{device.name}</strong><div className="meta">{device.workspace_name} · {device.platform.toUpperCase()}/{device.arch}{device.active_processes ? ` · ${device.active_processes} ACTIVE` : ""}</div></div>
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
      <header className="page-header"><div><p className="eyebrow">DEVICES</p><h1>Devices</h1></div>{canEnroll && <a className="or-button" href="/devices/enroll">ENROLL DEVICE</a>}</header>
      <DeviceRows devices={devices}/>
    </div> });
}

export function devicePage(user: User, workspaces: WorkspaceView[], device: DeviceView, processes: RemoteProcess[], rcVersion: string, sidebar: "open" | "closed") {
  const supportsProcess = device.capabilities.includes("process");
  const canOperate = device.role === "owner" || device.role === "operator";
  return htmlDocument({ title: device.name, user, workspaces, path: `/devices/${device.id}`, sidebar, scripts: ["live", "device"], body:
    <div className="page" data-device-page={device.id} data-supports-process={supportsProcess ? "true" : "false"}>
      <section className="device-overview">
        <header className="page-header device-header">
          <div><p className="eyebrow">DEVICE</p><h1>{device.name}</h1><p className="meta">{device.workspace_name} · {device.platform.toUpperCase()}/{device.arch}</p></div>
          <span id="device-status" className={`status${device.online ? " online" : ""}`}>{device.online ? "ONLINE" : `LAST SEEN ${relative(device.last_seen)}`}</span>
        </header>
        <dl className="facts">
          <div><dt>HOST</dt><dd>{device.hostname}</dd></div><div><dt>NODE</dt><dd id="node-agent">{device.agent_version}</dd></div>
          <div><dt>RC</dt><dd>{rcVersion}</dd></div>
        </dl>
      </section>
      <section className="content-section process-launch-section">
        <div className="section-heading"><div><SectionBadge index="01">Terminal</SectionBadge><h2>Remote shell</h2></div></div>
        {canOperate && <><button id="open-terminal" className="or-button" type="button" disabled={!device.online || !supportsProcess}>OPEN TERMINAL</button>
          <details className="advanced-launch"><summary>Advanced command</summary><form id="process-launch" className="process-form"><label>Working directory<input id="process-cwd" name="cwd" spellCheck={false} placeholder="~"/></label><label>Command<input id="process-command" name="command" spellCheck={false} defaultValue="sh" required/></label><button className="or-button" type="submit" disabled={!device.online || !supportsProcess}>START</button></form></details></>}
        {!canOperate && <p className="empty-state">Viewers can inspect this device but cannot start processes.</p>}
        <p id="process-error" className="error">{supportsProcess ? "" : <>This RC Node is too old for terminals. Run <code>ohrats-rc update</code> on the device.</>}</p>
      </section>
      <section className="content-section"><div className="section-heading"><div><SectionBadge index="02">Processes</SectionBadge><h2>History</h2></div></div>{device.role === "viewer" ? <p className="empty-state">Process history is available to operators and owners.</p> : <ProcessRows deviceId={device.id} processes={processes}/>}</section>
    </div> });
}

export function deleteDevicePage(user: User, workspaces: WorkspaceView[], device: DeviceView, sidebar: "open" | "closed", error = "") {
  return htmlDocument({ title: "Remove device", user, workspaces, path: `/devices/${device.id}/delete`, sidebar, body:
    <div className="page narrow-form-page"><header className="page-header"><div><p className="eyebrow">{device.workspace_name.toUpperCase()}</p><h1>Remove device</h1></div></header>
      <section className="content-section danger-section"><p>Remove <strong>{device.name}</strong> from this workspace?</p><p className="page-copy">The node is disconnected and its process history is deleted.</p>
        <form method="post" action={`/devices/${device.id}/delete`} className="actions"><button className="text-action danger-text" type="submit">REMOVE</button><a className="text-action" href={`/devices/${device.id}`}>CANCEL</a></form><p className="error">{error}</p>
      </section></div> });
}
