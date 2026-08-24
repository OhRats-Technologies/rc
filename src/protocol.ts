import { t, type Static } from "elysia";

const RequestId = t.String({ minLength: 1, maxLength: 100 });
const ProcessId = t.String({ minLength: 1, maxLength: 100 });
const DeviceId = t.String({ minLength: 1, maxLength: 100 });
const TerminalSize = t.Number({ minimum: 2, maximum: 500 });

export const BrowserCommandSchema = t.Union([
  t.Object({ type: t.Literal("ping") }),
  t.Object({
    type: t.Literal("process.start"), requestId: RequestId, deviceId: DeviceId,
    command: t.String({ minLength: 1, maxLength: 8192 }), cwd: t.Optional(t.String({ maxLength: 4096 })),
    cols: TerminalSize, rows: TerminalSize,
  }),
  t.Object({
    type: t.Literal("process.input"), requestId: t.Optional(RequestId), processId: ProcessId,
    data: t.String({ minLength: 1, maxLength: 65536 }),
  }),
  t.Object({
    type: t.Literal("process.resize"), requestId: t.Optional(RequestId), processId: ProcessId,
    cols: TerminalSize, rows: TerminalSize,
  }),
  t.Object({
    type: t.Literal("process.signal"), requestId: t.Optional(RequestId), processId: ProcessId,
    signal: t.Union([t.Literal("INT"), t.Literal("TERM"), t.Literal("KILL")]),
  }),
  t.Object({ type: t.Literal("node.update"), requestId: RequestId, deviceId: DeviceId }),
]);

export const RelayEventSchema = t.Object({
  kind: t.String(), workspaceId: t.Optional(t.Nullable(t.String())), deviceId: t.Optional(t.Nullable(t.String())),
  processId: t.Optional(t.Nullable(t.String())), audit: t.Optional(t.Boolean()), detail: t.Optional(t.Unknown()),
  at: t.Optional(t.Number()),
});

export const BrowserServerMessageSchema = t.Union([
  t.Object({ type: t.Literal("ready") }),
  t.Object({ type: t.Literal("pong") }),
  t.Object({ type: t.Literal("event"), event: RelayEventSchema }),
  t.Object({ type: t.Literal("response"), requestId: RequestId, ok: t.Literal(true), result: t.Optional(t.Unknown()) }),
  t.Object({ type: t.Literal("response"), requestId: RequestId, ok: t.Literal(false), error: t.String() }),
]);

export const AgentClientMessageSchema = t.Union([
  t.Object({
    type: t.Literal("hello"), agentVersion: t.String({ maxLength: 40 }), hostname: t.String({ maxLength: 255 }),
    platform: t.String({ maxLength: 40 }), arch: t.String({ maxLength: 40 }),
    capabilities: t.Array(t.String({ maxLength: 64 }), { maxItems: 32 }),
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
]);

export const AgentServerMessageSchema = t.Union([
  t.Object({
    type: t.Literal("process.start"), id: ProcessId, command: t.String({ maxLength: 8192 }),
    cwd: t.Nullable(t.String({ maxLength: 4096 })), cols: TerminalSize, rows: TerminalSize,
  }),
  t.Object({ type: t.Literal("process.input"), id: ProcessId, input: t.String({ maxLength: 65536 }) }),
  t.Object({ type: t.Literal("process.resize"), id: ProcessId, cols: TerminalSize, rows: TerminalSize }),
  t.Object({ type: t.Literal("process.signal"), id: ProcessId, signal: t.String({ maxLength: 32 }) }),
  t.Object({ type: t.Literal("node.update") }),
  t.Object({ type: t.Literal("node.remove") }),
]);

export type BrowserCommand = Static<typeof BrowserCommandSchema>;
export type BrowserServerMessage = Static<typeof BrowserServerMessageSchema>;
export type AgentClientMessage = Static<typeof AgentClientMessageSchema>;
export type AgentServerMessage = Static<typeof AgentServerMessageSchema>;
