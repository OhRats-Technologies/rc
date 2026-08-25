import type { ActionRunResult, ActionView } from "../../../src/actions";
import type { User } from "../../../src/core";
import type { DeviceView } from "../../../src/devices";
import type { WorkspaceView } from "../../../src/workspaces";
import { htmlDocument } from "../document";
import { SectionBadge } from "../components";

type Prefill = { workspaceId?: string; name?: string; description?: string; command?: string; cwd?: string; confirm?: boolean };

export function actionsPage(user: User, workspaces: WorkspaceView[], actions: ActionView[], sidebar: "open" | "closed") {
  const ownerWorkspace = workspaces.find(workspace => workspace.role === "owner");
  return htmlDocument({ title: "Actions", user, workspaces, path: "/actions", sidebar, body:
    <div className="page"><header className="page-header"><div><h1>Actions</h1></div>{ownerWorkspace && <a className="header-icon-button" href={`/actions/new?workspace=${ownerWorkspace.id}`} aria-label="New action" title="New action"><span className="ui-icon icon-plus" aria-hidden="true"/></a>}</header>
      <div className="data-list">{actions.length ? actions.map((action, index) => <a className="data-row workspace-row" href={`/actions/${action.id}`} key={action.id}><div><SectionBadge index={String(index + 1).padStart(2, "0")}>{action.name}</SectionBadge><div className="meta">{action.workspace_name}</div></div><span className="meta">{action.role.toUpperCase()}</span></a>) : <p className="empty-state">No saved actions yet.</p>}</div>
    </div> });
}

export function actionFormPage(user: User, workspaces: WorkspaceView[], sidebar: "open" | "closed", action?: ActionView | null, prefill: Prefill = {}, error = "") {
  const owners = workspaces.filter(workspace => workspace.role === "owner");
  const editing = Boolean(action), workspaceId = action?.workspace_id || prefill.workspaceId || owners[0]?.id || "";
  return htmlDocument({ title: editing ? `Edit ${action!.name}` : "New action", user, workspaces, path: editing ? `/actions/${action!.id}/edit` : "/actions/new", sidebar, body:
    <div className="page narrow-form-page"><header className="page-header"><div><p className="eyebrow">ACTIONS</p><h1>{editing ? "Edit action" : "New action"}</h1></div></header>
      <form method="post" action={editing ? `/actions/${action!.id}` : "/actions"} className="simple-form action-form">
        {!editing && <label>Workspace<select name="workspaceId" required>{owners.map(workspace => <option value={workspace.id} selected={workspace.id === workspaceId}>{workspace.name}</option>)}</select></label>}
        <label>Name<input name="name" defaultValue={action?.name || prefill.name || ""} required/></label>
        <label>Description<input name="description" defaultValue={action?.description || prefill.description || ""}/></label>
        <label>Command<textarea name="command" rows={4} spellCheck={false} required defaultValue={action?.command || prefill.command || ""}/></label>
        <label>Working directory<input name="cwd" spellCheck={false} placeholder="~" defaultValue={action?.cwd || prefill.cwd || ""}/></label>
        <label className="check-label"><input name="confirm" type="checkbox" value="1" defaultChecked={Boolean(action?.confirm || prefill.confirm)}/> Require explicit confirmation before running</label>
        <button className="or-button" type="submit">{editing ? "SAVE ACTION" : "CREATE ACTION"}</button><p className="error">{error}</p>
      </form>
    </div> });
}

export function actionPage(user: User, workspaces: WorkspaceView[], action: ActionView, devices: DeviceView[], sidebar: "open" | "closed", results: ActionRunResult[] = []) {
  const canRun = action.role === "owner" || action.role === "operator";
  return htmlDocument({ title: action.name, user, workspaces, path: `/actions/${action.id}`, sidebar, body:
    <div className="page"><header className="page-header"><div><p className="eyebrow">{action.workspace_name.toUpperCase()} / ACTION</p><h1>{action.name}</h1>{action.description && <p className="page-copy">{action.description}</p>}</div>{action.role === "owner" && <div className="page-header-actions"><a className="header-icon-button" href={`/actions/${action.id}/edit`} aria-label={`Edit ${action.name}`} title="Edit action"><span className="ui-icon icon-pencil" aria-hidden="true"/></a><button className="header-icon-button danger-icon-button" type="button" aria-label={`Delete ${action.name}`} title="Delete action" data-delete-kind="action" data-delete-name={action.name} data-delete-endpoint={`/api/v1/actions/${action.id}`} data-delete-redirect="/actions"><span className="ui-icon icon-trash" aria-hidden="true"/></button></div>}</header>
      <section className="content-section"><SectionBadge index="01">Command</SectionBadge><div className="or-copy-field" title={action.command}><span className="or-copy-prefix">$</span><code>{action.command}</code></div>{action.cwd && <span className="meta">CWD {action.cwd}</span>}</section>
      <section className="content-section"><div className="section-heading"><div><SectionBadge index="02">Run</SectionBadge><h2>Select devices</h2></div></div>
        {canRun ? <form method="post" action={`/actions/${action.id}/run`} className="action-run-form"><div className="device-selection">{devices.length ? devices.map(device => <label className="device-choice" key={device.id}><input type="checkbox" name="deviceId" value={device.id} disabled={!device.online}/><span><strong>{device.name}</strong><span className="meta">{device.online ? "ONLINE" : "OFFLINE"}</span></span></label>) : <p className="empty-state">No devices in this workspace.</p>}</div><button className="or-button" type="submit" disabled={!devices.some(device => device.online)}>{action.confirm ? "REVIEW & RUN" : "RUN ACTION"}</button></form> : <p className="empty-state">Viewers can inspect actions but cannot run them.</p>}
        {results.length > 0 && <div className="action-results">{results.map(result => <div className="setting-row" key={result.deviceId}><div><strong>{result.deviceName}</strong><div className="meta">{result.error ? result.error.toUpperCase() : "STARTED"}</div></div>{result.processId && <a className="text-action" href={`/devices/${result.deviceId}/processes/${result.processId}`}>OPEN →</a>}</div>)}</div>}
      </section>
    </div> });
}

export function actionConfirmPage(user: User, workspaces: WorkspaceView[], action: ActionView, devices: DeviceView[], selected: string[], sidebar: "open" | "closed") {
  const chosen = devices.filter(device => selected.includes(device.id));
  return htmlDocument({ title: `Run ${action.name}`, user, workspaces, path: `/actions/${action.id}`, sidebar, body:
    <div className="page narrow-form-page"><header className="page-header"><div><p className="eyebrow">{action.workspace_name.toUpperCase()} / ACTION</p><h1>Run {action.name}?</h1></div></header>
      <section className="content-section"><p className="page-copy">This will run on {chosen.length} {chosen.length === 1 ? "device" : "devices"}.</p><div className="or-copy-field" title={action.command}><span className="or-copy-prefix">$</span><code>{action.command}</code></div>
        <div className="data-list">{chosen.map(device => <div className="setting-row" key={device.id}><strong>{device.name}</strong><span className="meta">{device.online ? "ONLINE" : "OFFLINE"}</span></div>)}</div>
        <form method="post" action={`/actions/${action.id}/run`} className="actions"><input type="hidden" name="confirm" value="1"/>{chosen.map(device => <input key={device.id} type="hidden" name="deviceId" value={device.id}/>)}<button className="or-button" type="submit">RUN ACTION</button><a className="text-action" href={`/actions/${action.id}`}>CANCEL</a></form>
      </section>
    </div> });
}
