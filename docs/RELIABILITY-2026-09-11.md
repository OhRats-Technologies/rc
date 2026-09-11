# Reliability verification — 2026-09-11

Changes pushed to main: 41bc460 (readiness and execution identity), cf2ddb4
(reaping), 804e29d (stderr descriptors), db614d0 (failure classification),
917a0fc (substitution guards and status schema), aa19b60 (control framing
and failed-peer cleanup).

The old live Node repeatedly logged `outbound packet larger than maximum
message size` and reached a 14.2 GiB memory peak. A queued oversized message
survived reconnect, while ended peer connections were not explicitly closed.
The new framing regression exercises large messages in both directions over
real WebRTC, then starts another execution. Delivery never replays a start
whose outcome is uncertain.

Hosted deployment aa19b60 was healthy before installation. The enrolled Linux
Node received one service restart at 19:29:38 EDT. The unit remained enabled;
enrollment and RC Lock were preserved. Installation checked that the RC
cgroup contained only the Node before stopping it. No training files were
changed. New PID: 362768; automatic restart count: zero.

Live MCP process_run observations, through approximately 19:35 EDT:

| Behavior | Process ID | Result |
| --- | --- | --- |
| Sequential success | 2c5fa8c9-cbeb-49b4-af0f-19266eda7b8d | exited, code 0 |
| Missing executable | a1d585db-9a53-499c-97f2-43293b451248 | exited, code 1, stderr diagnostic, isError true |
| Nested substitution failure | 39a422fb-323d-49fe-b84d-418193701532 | exited, code 1, isError true; Node survived |
| Mixed stdout/stderr | 8c141e58-9bcf-4d8b-924e-97873f413377 | both labeled streams, cursor 8, exit 7, isError true |
| Stderr only | cd304a8b-6bcc-417b-a9c5-cae4cd30cf05 | stderr captured, cursor 4, exit 0 |
| 70 KB input/output fixture | 86ddf0be-5243-49df-9d1e-1c7a5a1ace21 | exited, code 0, 65536 bytes returned, outputPending true |
| Command after large frame | 6c668347-226a-439e-92df-34990ed10cd6 | exited, code 0 |
| Invalid substitution syntax | 80ccd7f0-b7c5-429f-b795-a05b72bbf028 | exited, code 1, stderr diagnostic, isError true |

After these calls, the Node had no child processes, including zombies;
memory was approximately 41 MiB. Its journal contained no new transport
errors or panics. machines_list reported online true and zero active processes.
This is a short live observation window, not proof that every disconnect or
possible panic has been eliminated.

The external connector still advertises process_status without deviceId.
A call for the large fixture at cursor 65536 returned exactly
`device is outside this MCP grant`, with INVALID_ARGUMENT and isError true,
despite supplying deviceId in the tool invocation. Consequently live continued
output reads remain blocked in this connector. The server descriptor requires
deviceId and processId; its schema regression and full HTTP MCP status tests
pass. Authorization was not weakened to accommodate stale connector metadata.
The connector also adds INVALID_ARGUMENT to nonzero process results; the
execution status, exit code, and isError distinguish those from loss.

Local validation passed: server unit/readiness/MCP/WebRTC tests, protocol
framing tests, shell tests, native runtime policy checks including reaping,
stderr replay and repeated substitution failures, relevant Clippy checks,
and source-size checks. Full cross-platform CI was still running at the time
of this record; its completion is not claimed here.
