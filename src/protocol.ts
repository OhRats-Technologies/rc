import { t, type Static } from "elysia";

const RequestId = t.String({ minLength: 1, maxLength: 100 });
const ProcessId = t.String({ minLength: 1, maxLength: 100 });
const DeviceId = t.String({ minLength: 1, maxLength: 100 });
const TerminalSize = t.Number({ minimum: 2, maximum: 500 });

export const BrowserCommandSchema = t.Union([
  t.Object({ type: t.Literal("ping") }),
  t.Object({
    type: t.Literal("process.allocate"), requestId: RequestId, deviceId: DeviceId, cols: TerminalSize, rows: TerminalSize,
  }),
  t.Object({ type: t.Literal("control.challenge"), requestId: RequestId, deviceId: DeviceId }),
  t.Object({ type: t.Literal("control.open"), requestId: RequestId, deviceId: DeviceId, challenge: t.String(),
    clientId: t.String(), publicKey: t.String(), signature: t.String() }),
  t.Object({ type: t.Literal("control.frame"), deviceId: DeviceId, sessionId: t.String(), sequence: t.Number(), ciphertext: t.String() }),
  t.Object({ type: t.Literal("control.close"), deviceId: DeviceId, sessionId: t.String() }),
  t.Object({ type: t.Literal("lock.sync"), requestId: RequestId, workspaceId: t.String(), clientId: t.String(), signature: t.String() }),
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
  t.Object({ type: t.Literal("control.frame"), sessionId: t.String(), sequence: t.Number(), ciphertext: t.String() }),
]);

export const AgentClientMessageSchema = t.Union([
  t.Object({
    type: t.Literal("hello"), agentVersion: t.String({ maxLength: 40 }), hostname: t.String({ maxLength: 255 }),
    platform: t.String({ maxLength: 40 }), arch: t.String({ maxLength: 40 }),
    capabilities: t.Array(t.String({ maxLength: 64 }), { maxItems: 32 }), transportPublicKey: t.Optional(t.String()), lockHash: t.Optional(t.String()),
  }),
  t.Object({ type: t.Literal("heartbeat") }),
  t.Object({ type: t.Literal("process.started"), id: ProcessId }),
  t.Object({ type: t.Literal("process.output"), id: ProcessId, output: t.String({ maxLength: 65536 }) }),
  t.Object({
    type: t.Literal("process.exit"), id: ProcessId, output: t.Optional(t.String({ maxLength: 65536 })),
    exitCode: t.Optional(t.Nullable(t.Integer())), signal: t.Optional(t.String({ maxLength: 32 })),
  }),
  t.Object({ type: t.Literal("node.update.ready"), agentVersion: t.Optional(t.String({ maxLength: 40 })) }),
  t.Object({ type: t.Literal("node.update.error"), output: t.Optional(t.String({ maxLength: 1024 })) }),
  t.Object({ type: t.Literal("control.challenge"), requestId: RequestId, challenge: t.String() }),
  t.Object({ type: t.Literal("control.ready"), requestId: RequestId, sessionId: t.String(), transportPublicKey: t.String(), ephemeralPublicKey: t.String(), signature: t.String() }),
  t.Object({ type: t.Literal("control.frame"), sessionId: t.String(), sequence: t.Number(), ciphertext: t.String() }),
  t.Object({ type: t.Literal("control.error"), requestId: t.Optional(RequestId), output: t.String() }),
  t.Object({ type: t.Literal("lock.state"), lockHash: t.String() }),
]);

export const AgentServerMessageSchema = t.Union([
  t.Object({ type: t.Literal("lock.bootstrap"), snapshot: t.String() }),
  t.Object({ type: t.Literal("lock.sync"), snapshot: t.String(), grant: t.String(), credentialId: t.String(), assertion: t.String(), signature: t.String() }),
  t.Object({ type: t.Literal("control.challenge"), requestId: RequestId }),
  t.Object({ type: t.Literal("control.open"), requestId: RequestId, challenge: t.String(), clientId: t.String(), grant: t.String(),
    credentialId: t.String(), assertion: t.String(), publicKey: t.String(), signature: t.String() }),
  t.Object({ type: t.Literal("control.frame"), sessionId: t.String(), sequence: t.Number(), ciphertext: t.String() }),
  t.Object({ type: t.Literal("control.close"), sessionId: t.String() }),
]);

export type BrowserCommand = Static<typeof BrowserCommandSchema>;
export type BrowserServerMessage = Static<typeof BrowserServerMessageSchema>;
export type AgentClientMessage = Static<typeof AgentClientMessageSchema>;
export type AgentServerMessage = Static<typeof AgentServerMessageSchema>;
