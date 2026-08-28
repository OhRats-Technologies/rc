# RC swarm compact protocol (SC2)

SC2 is an LLM-native coordination language. Its purpose is to reduce the model
input/output spent rereading and writing parallel-work conversations. Agents
write SC2 directly; they do not write prose and ask a script to compress it.

Live state is under the user-state path returned by:

```sh
python3 scripts/swarm.py path
```

The default is `~/.local/state/rc-swarm/<repo-id>/`. Linked worktrees of this
checkout resolve the same directory. `.git/swarm` is retired. The live store is
working state, not history: prune resolved topics and inactive workers after
their durable outcome exists in code/tests/docs/commits.

## Records

SC2 uses file context instead of repeating metadata on every line.

Agent file `agents/<agent-id>.s2`:

```text
K BODY
```

The agent ID is implicit from the filename.

Thread file `threads/<topic>.s2`:

```text
FROM K BODY
```

`FROM` is either a stable agent ID or a thread-local alias. The thread name is
implicit from the filename. File order is chronology; timestamps are omitted
unless time itself matters to the work.

Optional thread-local aliases are declared once:

```text
@ u=sol-updater-4c91 w=web-migrate-a73f n=node-runtime-8d31 pm=C/package-manager kp=K/platform
```

Aliases exist to remove repeated long identifiers. Agents may add compact local
aliases when a term repeats enough to pay for the declaration. Do not require a
global dictionary for ordinary domain shorthand.

Kinds:

```text
c claim       a ack          p proposal      q question
r response    s status       b blocker       x conflict
w warning     t test         u published     h handoff
d resolved    z cancelled
```

## Body language

`BODY` is compact agent language, not a fixed key/value schema. Prefer short
clauses separated by `;`. These conventions are useful, not mandatory syntax:

```text
+x        add/own/enable x
-x        exclude/avoid/remove x
>x        next/handoff/transition to x
?x        need/query x
!x        important failure/risk x
x:y       relation or attribute
ok:x      verified/passed x
fail:x    failed x
a,b,c     compact set/list
```

Repository path prefixes may be shortened as:

```text
K/ kernel/       C/ components/     W/ wit/
R/ crates/       S/ scripts/        G/ .github/
```

Common domain shorthand such as `upd`, `tr`, `proc`, `web`, `wit`, `kern`,
`rel`, `ci`, `e2e`, `tst`, and `nxt` may be used directly when peers can infer
it from the topic. Thread-local definitions are appropriate when an abbreviation
would otherwise be ambiguous.

Example thread:

```text
@ u=sol-updater-4c91 w=web-migrate-a73f pm=C/package-manager kp=K/platform
u c +upd; +pm,updater-host,kp; -tr,-web; >tst
w a wit overlap:none; preserve:updater-host
u s hard+; tst:rust/foc run; >smoke,rebase,push
u t ok:kern14,pm2,cli7; smoke:run
u h c:abc1234; >main-review
```

An agent reading that thread should understand it directly. Human translation
can be requested from an agent or obtained with the optional decoder; decoder
round-tripping is not a protocol requirement.

## Measured compression

`scripts/benchmark-swarm-protocol.py` compares four representative live RC
coordination records. The current corpus is 1119 bytes as SC1 and 339 bytes as
direct SC2: **69.7% fewer bytes**.

A controlled Codex run used the same fixed instruction and output (`OK`) with
an empty-state baseline to remove the model/system prompt overhead. The four
coordination records added 243 input tokens as SC1 and 98 as SC2: **59.7% fewer
coordination input tokens**. A separate read-only model comprehension check
correctly recovered owner, exclusions, passed/running tests, next actions, and
the absence of a blocker directly from SC2 without a decoder.

## Writing

Canonical direct writing uses `append`, which only validates one-line input,
file-locks it, and appends it unchanged:

```sh
python3 scripts/swarm.py append --thread updater-platform \
  'u s hard+; tst:rust/foc run; >smoke,rebase,push'

python3 scripts/swarm.py append --agent sol-updater-4c91 \
  's hard+; >smoke,push'
```

`post` remains a transition shim for workers that have not yet switched to
direct SC2. It emits SC2, not SC1. New work should use `append`.

Useful commands:

```sh
python3 scripts/swarm.py list
python3 scripts/swarm.py read --thread updater-platform --tail 30
python3 scripts/swarm.py prune --thread updater-platform
python3 scripts/swarm.py decode 'u s hard+; >smoke'
```

## Rules

1. Every worker has one stable unique ID and an isolated worktree/branch.
2. Read relevant agent/topic state before overlapping shared files.
3. Active files are append-only and file-locked. Do not rewrite another
   worker's live message; supersede it with a new one.
4. Preserve operational meaning needed by peers: ownership, conflicts,
   blockers, tests, handoffs, and important deadlines must remain clear.
5. Optimize for model tokens and peer comprehension, not human prose quality or
   byte-perfect reversibility.
6. Do not put secrets, credentials, private keys, or tokens in coordination.
7. Delete resolved threads/stale agent files instead of building an archive.
