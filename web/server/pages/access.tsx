import type { User } from "../../../src/core";
import type { InviteView, MemberView } from "../../../src/workspace-access";
import type { WorkspaceView } from "../../../src/workspaces";
import { htmlDocument } from "../document";
import { relative, until } from "../format";
import { SectionBadge } from "../components";

type InviteResult = { url: string; role: "operator" | "viewer" } | null;

export function accessPage(user: User, workspaces: WorkspaceView[], workspace: WorkspaceView, members: MemberView[], invites: InviteView[], sidebar: "open" | "closed", result: InviteResult = null, error = "") {
  return htmlDocument({ title: `${workspace.name} access`, user, workspaces, path: `/workspaces/${workspace.id}/access`, sidebar, scripts: result ? ["copy"] : [], body:
    <div className="page"><header className="page-header"><div><p className="eyebrow">{workspace.name.toUpperCase()} / ACCESS</p><h1>Manage access</h1></div></header>
      <section className="content-section"><div className="section-heading"><div><SectionBadge index="01">Invite</SectionBadge><h2>Invite an operator or viewer</h2></div></div>
        <form method="post" action={`/workspaces/${workspace.id}/invites`} className="inline-form compact-inline-form">
          <label>Role<select name="role"><option value="operator">Operator</option><option value="viewer">Viewer</option></select></label>
          <button className="or-button" type="submit">CREATE INVITE</button>
        </form>
        {result && <div className="invite-link"><span className="meta">{result.role.toUpperCase()} INVITE · SHOWN ONCE</span><div className="or-copy-field" data-copy-value={result.url} title={result.url}><code>{result.url}</code><button className="or-copy-button copy-value" type="button" aria-label="Copy invite link"><span className="or-copy-icon" aria-hidden="true"/></button></div></div>}
        <p className="error">{error}</p>
      </section>
      <section className="content-section"><div className="section-heading"><div><SectionBadge index="02">Members</SectionBadge><h2>{members.length} people</h2></div></div>
        <div className="settings-list">{members.map(member => <div className="setting-row access-row" key={member.user_id}>
          <div><strong>{member.name}{member.user_id === user.id ? " (you)" : ""}</strong><div className="meta">JOINED {relative(member.joined_at)}</div></div>
          <div className="row-actions"><form method="post" action={`/workspaces/${workspace.id}/members/${member.user_id}/role`} className="role-form"><select name="role" defaultValue={member.role} aria-label={`Role for ${member.name}`}><option value="owner">Owner</option><option value="operator">Operator</option><option value="viewer">Viewer</option></select><button className="text-button" type="submit">SAVE</button></form>
            {member.user_id !== user.id && <form method="post" action={`/workspaces/${workspace.id}/members/${member.user_id}/remove`}><button className="text-button danger-text" type="submit">REMOVE</button></form>}
          </div>
        </div>)}</div>
      </section>
      <section className="content-section"><div className="section-heading"><div><SectionBadge index="03">Pending</SectionBadge><h2>Invitations</h2></div></div>
        <div className="settings-list">{invites.length ? invites.map(invite => <div className="setting-row" key={invite.id}><div><strong>{invite.role.toUpperCase()}</strong><div className="meta">EXPIRES IN {until(invite.expires_at)}</div></div><form method="post" action={`/workspaces/${workspace.id}/invites/${invite.id}/revoke`}><button className="text-button" type="submit">REVOKE</button></form></div>) : <p className="empty-state">No pending invitations.</p>}</div>
      </section>
    </div> });
}
