# RC parallel-work board

This file is the coordination surface for agents and humans working in this
repository at the same time. Read it before editing, then add a dated message
before claiming an area. Keep messages concise and append-only; update a claim
with a new message rather than rewriting another worker's note.

Rules for parallel work:

- Check `git status`, `git log -1`, and this board before each edit batch.
- Claim directories or concrete files, not broad goals. Avoid files claimed by
  another active worker unless they explicitly hand them off here.
- Never reset, clean, amend, rebase, or force-push work you did not create.
- Prefer small commits with focused ownership. Push completed commits so other
  workers can merge/rebase normally.
- When a shared WIT contract must change, post the proposed interface here
  first and list every known consumer.
- Record blockers, handoffs, test results, and published releases here. Do not
  use this board for secrets or private credentials.
- Transport work and non-transport component migration should remain separate
  whenever possible. Coordinate at WIT/profile boundaries instead of editing
  one another's implementations.

## Messages

### 2026-08-28 17:55 EDT — GPT-5.6 component-migration worker

Claiming non-transport component migration: `components/webui-shell`, new
HTTP/API/MCP/SSH components, their WIT contracts, kernel HTTP/session adapters,
profiles, migration tests, docs, and removal of superseded native server code.
I will not edit WebRTC/TURN transport implementations or transport-specific
protocol files unless a shared interface change is posted here first. The
parallel transport worker can claim those files below. My first deliverable is
an exact component-owned landing/docs WebUI plus a canonical-server profile,
then identity-backed authenticated HTTP routes, API, MCP, and SSH in vertical
slices.
