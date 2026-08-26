import { t, type Static } from "elysia";

const RequestId = t.String({ minLength: 1, maxLength: 100 });
const ProcessId = t.String({ minLength: 1, maxLength: 100 });
const DeviceId = t.String({ minLength: 1, maxLength: 100 });
const SessionId = t.String({ minLength: 1, maxLength: 100 });
const ControlSequence = t.Integer({ minimum: 1 });
const ControlCiphertext = t.String({ minLength: 1, maxLength: 1_500_000 });
const ProcessData = t.String({ minLength: 1, maxLength: 131_072, pattern: "^[A-Za-z0-9_-]+$" });
const TerminalSize = t.Number({ minimum: 2, maximum: 500 });
const ProcessTerminal = t.Object({ cols: TerminalSize, rows: TerminalSize, term: t.Optional(t.String({ maxLength: 128 })) });
const IceServerSchema = t.Object({
  urls: t.Array(t.String({ minLength: 1, maxLength: 512 }), { minItems: 1, maxItems: 16 }),
  username: t.Optional(t.String({ maxLength: 512 })), credential: t.Optional(t.String({ maxLength: 512 })),
});
const IceCandidateSummarySchema = t.Object({
  host: t.Integer({ minimum: 0, maximum: 64 }),
  srflx: t.Integer({ minimum: 0, maximum: 64 }),
  relay: t.Integer({ minimum: 0, maximum: 64 }),
  udp: t.Integer({ minimum: 0, maximum: 64 }),
  tcp: t.Integer({ minimum: 0, maximum: 64 }),
});

export const BrowserCommandSchema = t.Union([
  t.Object({ type: t.Literal("ping") }),
  t.Object({
    type: t.Literal("process.allocate"), requestId: RequestId, deviceId: DeviceId, terminal: t.Optional(t.Boolean()),
    cols: TerminalSize, rows: TerminalSize,
  }),
  t.Object({ type: t.Literal("control.challenge"), requestId: RequestId, deviceId: DeviceId }),
  t.Object({ type: t.Literal("control.open"), requestId: RequestId, deviceId: DeviceId, challenge: t.String(),
    clientId: t.String(), publicKey: t.String(), signature: t.String() }),
  t.Object({ type: t.Literal("control.webrtc"), requestId: RequestId, deviceId: DeviceId, sessionId: SessionId,
    sdp: t.String({ minLength: 1, maxLength: 131072 }) }),
  t.Object({ type: t.Literal("control.transport"), deviceId: DeviceId, sessionId: SessionId,
    transport: t.Union([t.Literal("webrtc"), t.Literal("relay")]),
    reason: t.Optional(t.String({ maxLength: 200 })),
    iceState: t.Optional(t.String({ maxLength: 40 })), connectionState: t.Optional(t.String({ maxLength: 40 })),
    localCandidates: t.Optional(IceCandidateSummarySchema), remoteCandidates: t.Optional(IceCandidateSummarySchema),
    selected: t.Optional(t.Object({
      localType: t.Optional(t.String({ maxLength: 20 })), remoteType: t.Optional(t.String({ maxLength: 20 })),
      protocol: t.Optional(t.String({ maxLength: 20 })),
    })),
  }),
  t.Object({ type: t.Literal("control.frame"), deviceId: DeviceId, sessionId: SessionId, sequence: ControlSequence, ciphertext: ControlCiphertext }),
  t.Object({ type: t.Literal("control.close"), deviceId: DeviceId, sessionId: SessionId }),
  t.Object({ type: t.Literal("lock.sync"), requestId: RequestId, workspaceId: t.String(), clientId: t.String(),
    transitions: t.Array(t.Object({ fromHash: t.String({ minLength: 64, maxLength: 64 }), generation: t.Integer({ minimum: 0 }), signature: t.String() }), { minItems: 1, maxItems: 100 }) }),
]);

export const RCEventSchema = t.Object({
  kind: t.String(), workspaceId: t.Optional(t.Nullable(t.String())), deviceId: t.Optional(t.Nullable(t.String())),
  processId: t.Optional(t.Nullable(t.String())), audit: t.Optional(t.Boolean()), detail: t.Optional(t.Unknown()),
  at: t.Optional(t.Number()),
});

export const BrowserServerMessageSchema = t.Union([
  t.Object({ type: t.Literal("ready") }),
  t.Object({ type: t.Literal("pong") }),
  t.Object({ type: t.Literal("event"), event: RCEventSchema }),
  t.Object({ type: t.Literal("response"), requestId: RequestId, ok: t.Literal(true), result: t.Optional(t.Unknown()) }),
  t.Object({ type: t.Literal("response"), requestId: RequestId, ok: t.Literal(false), error: t.String() }),
  t.Object({ type: t.Literal("control.frame"), sessionId: SessionId, sequence: ControlSequence, ciphertext: ControlCiphertext }),
]);

export const AgentClientMessageSchema = t.Union([
  t.Object({
    type: t.Literal("hello"), agentVersion: t.String({ maxLength: 40 }), hostname: t.String({ maxLength: 255 }),
    platform: t.String({ maxLength: 40 }), arch: t.String({ maxLength: 40 }),
    capabilities: t.Array(t.String({ maxLength: 64 }), { maxItems: 32 }), transportPublicKey: t.Optional(t.String()), lockHash: t.Optional(t.String()),
    lockGeneration: t.Optional(t.Integer({ minimum: 0 })),
  }),
  t.Object({ type: t.Literal("heartbeat") }),
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
  t.Object({ type: t.Literal("control.frame"), sessionId: SessionId, sequence: ControlSequence, ciphertext: ControlCiphertext }),
  t.Object({ type: t.Literal("control.error"), requestId: t.Optional(RequestId), output: t.String() }),
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
  t.Object({ type: t.Literal("control.frame"), sessionId: SessionId, sequence: ControlSequence, ciphertext: ControlCiphertext }),
  t.Object({ type: t.Literal("control.close"), sessionId: SessionId }),
]);

export type BrowserCommand = Static<typeof BrowserCommandSchema>;
export type BrowserServerMessage = Static<typeof BrowserServerMessageSchema>;
export type AgentClientMessage = Static<typeof AgentClientMessageSchema>;
export type AgentServerMessage = Static<typeof AgentServerMessageSchema>;
