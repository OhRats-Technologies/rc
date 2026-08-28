# RC swarm compact protocol (SC1)

SC1 is a compact, explicit coordination format for agents and humans. It is
not encryption, steganography, or an unauditable side channel. Every message is
plain text and reversibly decodes with `python3 scripts/swarm.py decode`.

Live coordination is stored under Git's shared common directory so every
linked worktree sees the same files:

- `.git/swarm/agents/<stable-id>.md` — one worker's status, claims, and handoffs.
- `.git/swarm/threads/<topic>.md` — cross-worker discussion for one topic.
- `.git/swarm/PROTOCOL.md` — local copy of this protocol.

`BOARD.md` is historical and read-only. Do not allocate new work there.

## Wire form

Each new entry is one line:

```text
SC1|UTC|AGENT|KIND|SCOPE|REFS|PAYLOAD
```

Fields are separated by `|`. Literal `%`, `|`, carriage return, and newline in
a field are encoded as `%25`, `%7C`, `%0D`, and `%0A`. Empty scope or refs are
written as `-`. The helper performs encoding and decoding.

Kinds:

| Code | Meaning |
| --- | --- |
| `CLM` | task or ownership claim |
| `ACK` | acknowledgement |
| `PRP` | proposal |
| `QRY` | question |
| `RSP` | response |
| `STA` | progress/status |
| `BLK` | blocker |
| `CFT` | conflict |
| `WRN` | warning/deadline |
| `TST` | test result |
| `PUB` | commit/push/release published |
| `HOF` | handoff |
| `RES` | resolved/closed |
| `CAN` | cancelled |

`SCOPE` is a comma-separated list of paths or domain names. `REFS` is a
comma-separated list such as `@agent-id`, `#thread-name`, `commit:abc1234`, or
`path:kernel/src/node`. `PAYLOAD` is compact text; preferred keys are
`own=`, `avoid=`, `next=`, `need=`, `why=`, `test=`, `commit=`, `branch=`,
`wt=`, and `ddl=`.

Example:

```text
SC1|20260828T230000Z|node-runtime-8d31|CLM|kernel/src/node,wit/deps/transport|@web-migrate-a73f,#coord-v2|own=node transport+process policy;avoid=web/http;next=release gates
```

Decoded:

```text
[2026-08-28T23:00:00Z] node-runtime-8d31 CLAIM
scope: kernel/src/node,wit/deps/transport
refs: @web-migrate-a73f,#coord-v2
message: own=node transport+process policy;avoid=web/http;next=release gates
```

## Rules

1. Use one stable, unique agent ID and one isolated worktree/branch.
2. Append only. Never edit or delete another worker's prior entry.
3. Write routine status to your agent file; use a topic thread for shared
   contracts, questions, conflicts, warnings, and decisions.
4. Read the relevant agent and thread files before shared edits.
5. Keep payloads concise, but never omit ownership, conflicts, blockers, or
   safety-relevant context merely to save bytes.
6. Do not put secrets, credentials, private keys, or tokens in coordination.
7. Any compact message must round-trip through the documented SC1 decoder.
   Undocumented hidden encodings are not coordination protocol.

Common commands:

```sh
python3 scripts/swarm.py init
python3 scripts/swarm.py list
python3 scripts/swarm.py read --thread coord-v2 --tail 40
python3 scripts/swarm.py post --agent node-runtime-8d31 --kind CLM \
  --thread coord-v2 --scope kernel/src/node --refs @web-migrate-a73f \
  'own=node runtime;next=tests'
python3 scripts/swarm.py decode 'SC1|...'
```
