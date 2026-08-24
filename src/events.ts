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

export function eventStream(userId: string) {
  const encoder = new TextEncoder();
  let subscriberId = 0;
  let keepalive: ReturnType<typeof setInterval> | null = null;
  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      const write = (value: string) => {
        try { controller.enqueue(encoder.encode(value)); } catch {}
      };
      subscriberId = nextSubscriber++;
      subscribers.set(subscriberId, {
        userId,
        send: event => write(`data: ${JSON.stringify(event)}\n\n`),
      });
      write("retry: 1500\n\n");
      keepalive = setInterval(() => write(": keepalive\n\n"), 15_000);
    },
    cancel() {
      subscribers.delete(subscriberId);
      if (keepalive) clearInterval(keepalive);
    },
  });
  return new Response(stream, {
    headers: {
      "content-type": "text/event-stream; charset=utf-8",
      "cache-control": "no-store",
      "connection": "keep-alive",
      "x-accel-buffering": "no",
    },
  });
}
