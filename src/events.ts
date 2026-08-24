import { q } from "./db";

export type RelayEvent = {
  kind: string;
  workspaceId?: string | null;
  deviceId?: string | null;
  sessionId?: string | null;
  jobId?: string | null;
  audit?: boolean;
  detail?: unknown;
  at?: number;
};

type Subscriber = {
  userId: string;
  send: (event: RelayEvent) => void;
};

const subscribers = new Map<number, Subscriber>();
let nextSubscriber = 1;

function canReceive(userId: string, event: RelayEvent) {
  if (!event.workspaceId) return false;
  return !!q<any>("SELECT 1 ok FROM workspace_members WHERE workspace_id=? AND user_id=?")
    .get(event.workspaceId, userId);
}

export function publishEvent(event: RelayEvent) {
  const payload = { ...event, at: event.at || Date.now() };
  for (const subscriber of subscribers.values()) {
    if (canReceive(subscriber.userId, payload)) subscriber.send(payload);
  }
}

export function eventStream(req: Request, userId: string) {
  const encoder = new TextEncoder();
  const stream = new TransformStream<Uint8Array, Uint8Array>();
  const writer = stream.writable.getWriter();
  const subscriberId = nextSubscriber++;
  let closed = false;
  let keepalive: ReturnType<typeof setInterval>;
  const cleanup = () => {
    if (closed) return;
    closed = true;
    subscribers.delete(subscriberId);
    clearInterval(keepalive);
    void writer.close().catch(() => {});
  };
  const write = (value: string) => {
    if (closed) return;
    void writer.write(encoder.encode(value)).catch(cleanup);
  };
  subscribers.set(subscriberId, {
    userId,
    send: event => write(`data: ${JSON.stringify(event)}\n\n`),
  });
  write("retry: 1500\n\n");
  keepalive = setInterval(() => write(": keepalive\n\n"), 15_000);
  req.signal.addEventListener("abort", cleanup, { once: true });
  return new Response(stream.readable, {
    headers: {
      "content-type": "text/event-stream; charset=utf-8",
      "cache-control": "no-store",
      "connection": "keep-alive",
      "x-accel-buffering": "no",
    },
  });
}
