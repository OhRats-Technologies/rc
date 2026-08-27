import { t, type Static } from "elysia";

const RequestId = t.String({ minLength: 1, maxLength: 100 });
const ProcessId = t.String({ minLength: 1, maxLength: 100 });
const SessionId = t.String({ minLength: 1, maxLength: 100 });
const ProcessData = t.String({ minLength: 1, maxLength: 131_072, pattern: "^[A-Za-z0-9_-]+$" });
const TerminalSize = t.Number({ minimum: 2, maximum: 500 });
const ProcessTerminal = t.Object({ cols: TerminalSize, rows: TerminalSize, term: t.Optional(t.String({ maxLength: 128 })) });
const IceServerSchema = t.Object({
  urls: t.Array(t.String({ minLength: 1, maxLength: 512 }), { minItems: 1, maxItems: 16 }),
  username: t.Optional(t.String({ maxLength: 512 })), credential: t.Optional(t.String({ maxLength: 512 })),
});

export const AgentClientMessageSchema = t.Union([
  t.Object({
    type: t.Literal("hello"), agentVersion: t.String({ maxLength: 40 }), hostname: t.String({ maxLength: 255 }),
    platform: t.String({ maxLength: 40 }), arch: t.String({ maxLength: 40 }),
    capabilities: t.Array(t.String({ maxLength: 64 }), { maxItems: 32 }), transportPublicKey: t.Optional(t.String()), lockHash: t.Optional(t.String()),
    lockGeneration: t.Optional(t.Integer({ minimum: 0 })),
  }),
  t.Object({ type: t.Literal("heartbeat") }),
  t.Object({ type: t.Literal("process.sync"), ids: t.Array(ProcessId, { maxItems: 1024 }) }),
  t.Object({ type: t.Literal("process.started"), id: ProcessId }),
  t.Object({ type: t.Literal("process.start.request"), id: ProcessId, userId: t.String({ minLength: 1, maxLength: 100 }) }),
  t.Object({ type: t.Literal("process.stdout"), id: ProcessId, data: ProcessData }),
  t.Object({ type: t.Literal("process.stderr"), id: ProcessId, data: ProcessData }),
  t.Object({
    type: t.Literal("process.exit"), id: ProcessId, output: t.Optional(t.String({ maxLength: 65536 })),
    exitCode: t.Optional(t.Nullable(t.Integer())), signal: t.Optional(t.String({ maxLength: 32 })),
  }),
  t.Object({ type: t.Literal("node.update.ready"), agentVersion: t.Optional(t.String({ maxLength: 40 })) }),
  t.Object({ type: t.Literal("node.update.error"), output: t.Optional(t.String({ maxLength: 1024 })) }),
  t.Object({ type: t.Literal("control.challenge"), requestId: RequestId, challenge: t.String() }),
  t.Object({ type: t.Literal("control.ready"), requestId: RequestId, sessionId: SessionId, transportPublicKey: t.String(), ephemeralPublicKey: t.String(), signature: t.String() }),
  t.Object({ type: t.Literal("control.webrtc.ready"), requestId: RequestId, sessionId: SessionId, sdp: t.String({ minLength: 1, maxLength: 131072 }) }),
  t.Object({ type: t.Literal("control.error"), requestId: t.Optional(RequestId), output: t.String() }),
  t.Object({ type: t.Literal("control.closed"), sessionId: SessionId }),
  t.Object({ type: t.Literal("lock.state"), lockHash: t.String(), lockGeneration: t.Integer({ minimum: 0 }) }),
]);

export const AgentServerMessageSchema = t.Union([
  t.Object({ type: t.Literal("lock.bootstrap"), snapshot: t.String() }),
  t.Object({ type: t.Literal("lock.sync"), snapshot: t.String(), previousHash: t.String(), previousGeneration: t.Integer({ minimum: 0 }),
    grant: t.String(), credentialId: t.String(), assertion: t.String(), signature: t.String() }),
  t.Object({ type: t.Literal("process.permit"), id: ProcessId, userId: t.String({ minLength: 1, maxLength: 100 }) }),
  t.Object({ type: t.Literal("mcp.process.start"), id: ProcessId, userId: t.String({ minLength: 1, maxLength: 100 }),
    command: t.String({ minLength: 1, maxLength: 8192 }), cwd: t.Optional(t.String({ maxLength: 4096 })),
    mcpGrant: t.String({ minLength: 1, maxLength: 65536 }), mcpSignature: t.String({ minLength: 1, maxLength: 256 }),
    grant: t.String({ minLength: 1, maxLength: 8192 }), credentialId: t.String({ minLength: 1, maxLength: 2048 }),
    assertion: t.String({ minLength: 1, maxLength: 16384 }) }),
  t.Object({ type: t.Literal("ssh.process.start"), id: ProcessId, sessionId: SessionId, userId: t.String({ minLength: 1, maxLength: 100 }),
    command: t.String({ minLength: 1, maxLength: 8192 }), cwd: t.Optional(t.String({ maxLength: 4096 })), terminal: t.Optional(ProcessTerminal),
    grant: t.String({ minLength: 1, maxLength: 8192 }), credentialId: t.String({ minLength: 1, maxLength: 2048 }), assertion: t.String({ minLength: 1, maxLength: 16384 }) }),
  t.Object({ type: t.Literal("ssh.process.stdin"), id: ProcessId, sessionId: SessionId, data: ProcessData }),
  t.Object({ type: t.Literal("ssh.process.stdin.close"), id: ProcessId, sessionId: SessionId }),
  t.Object({ type: t.Literal("ssh.process.resize"), id: ProcessId, sessionId: SessionId, cols: TerminalSize, rows: TerminalSize }),
  t.Object({ type: t.Literal("ssh.process.signal"), id: ProcessId, sessionId: SessionId, signal: t.String({ minLength: 1, maxLength: 32 }) }),
  t.Object({ type: t.Literal("control.challenge"), requestId: RequestId }),
  t.Object({ type: t.Literal("control.open"), requestId: RequestId, challenge: t.String(), clientId: t.String(), grant: t.String(),
    credentialId: t.String(), assertion: t.String(), publicKey: t.String(), signature: t.String() }),
  t.Object({ type: t.Literal("control.webrtc"), requestId: RequestId, sessionId: SessionId, sdp: t.String({ minLength: 1, maxLength: 131072 }),
    iceServers: t.Array(IceServerSchema, { maxItems: 8 }) }),
  t.Object({ type: t.Literal("control.close"), sessionId: SessionId }),
]);

export type AgentClientMessage = Static<typeof AgentClientMessageSchema>;
export type AgentServerMessage = Static<typeof AgentServerMessageSchema>;
