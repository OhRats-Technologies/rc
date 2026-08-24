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
  const subscriberId = nextSubscriber++;
  const queue: string[] = ["retry: 1500\n\n"];
  let wake: (() => void) | null = null;
  let closed = false;
  let keepalive: ReturnType<typeof setInterval>;
  const cleanup = () => {
    if (closed) return;
    closed = true;
    subscribers.delete(subscriberId);
    clearInterval(keepalive);
    wake?.();
    wake = null;
  };
  const push = (value: string) => {
    if (closed) return;
    queue.push(value);
    wake?.();
    wake = null;
  };
  subscribers.set(subscriberId, {
    userId,
    send: event => push(`data: ${JSON.stringify(event)}\n\n`),
  });
  keepalive = setInterval(() => push(": keepalive\n\n"), 15_000);
  const stream = new ReadableStream<Uint8Array>({
    async pull(controller) {
      while (!queue.length && !closed) await new Promise<void>(resolve => { wake = resolve; });
      if (queue.length) controller.enqueue(encoder.encode(queue.shift()!));
      else if (closed) controller.close();
    },
    cancel: cleanup,
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
