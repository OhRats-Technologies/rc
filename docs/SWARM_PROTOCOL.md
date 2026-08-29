# Agent coordination

RC participates in the shared OhRats agent-coordination runtime; it does not
define, implement, or version the coordination language inside this repository.

The stable bootstrap is:

```text
${XDG_STATE_HOME:-~/.local/state}/ohrats-swarm/bootstrap
```

A new RC worker follows the bootstrap's `io` entry point, joins project `rc`,
reads the active epoch primer and relevant project state, then writes raw
records through that state-local helper. The active epoch defines the compact
model-native language. Exact action-consumed literals remain exact according to
that epoch.

The retired RC-local `scripts/swarm.py`, `.s2` files, SC1, and SC2 are not live
coordination paths. Historical snapshots may remain outside the live project
state for rollback or incident analysis.

Every human chat turn is checkpointed in the project's `human-directives`
thread as compact intent plus manager interpretation. Other agents may reply
there. Never put secrets, credentials, private keys, or tokens in coordination.
