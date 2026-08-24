# Relay v1

- [x] Fresh product repository
- [x] Freeze domain model: users, workspaces, fleets, devices, sessions, jobs
- [x] Replace prototype application architecture
- [x] Add persistent SQLite schema and migrations
- [x] Add passkey-only setup, login, logout, invites, workspace membership
- [x] Add workspace and fleet management
- [x] Add signed device identity and enrollment tokens
- [x] Add agent heartbeat, reconnect, capability advertisement
- [x] Add sessions and jobs
- [x] Add API v1 authentication and personal API tokens
- [x] Replace prototype UI with OhRats design system
- [x] Build downloadable Linux/macOS agents
- [x] Test multiple agents end-to-end
- [x] Deploy `relay.ohrats.party` with persistent Coolify volume
- [x] Verify persistence across restart
- [x] Verify production end-to-end
- [x] Replace browser polling with authenticated SSE
- [x] Stream command output and acknowledge job execution
- [x] Make node shutdown cancel active command process groups
- [x] Brand the node CLI as `ohrats-relay` with self-uninstall
- [x] Expire half-open nodes when heartbeats stop
- [x] Fail ambiguous in-flight jobs after control-plane restart instead of replaying them
- [x] Verify fleet, activity, presence, and command output update live without page refresh/polling
- [x] Persist active console sessions across reloads with explicit fresh-session control
- [x] Replace the generic shell card with the Relay node control surface

