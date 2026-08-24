import { q } from "./db";

export type RelayEvent = {
  kind: string;
  workspaceId?: string | null;
  deviceId?: string | null;
  processId?: string | null;
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
  const role = q<{ role: string }>("SELECT role FROM workspace_members WHERE workspace_id=? AND user_id=?")
    .get(event.workspaceId, userId)?.role;
  if (!role) return false;
  if (event.kind.startsWith("process.") && !event.audit) return role === "owner" || role === "operator";
  return true;
}

export function publishEvent(event: RelayEvent) {
  const payload = { ...event, at: event.at || Date.now() };
  for (const subscriber of subscribers.values()) {
    if (canReceive(subscriber.userId, payload)) subscriber.send(payload);
  }
}

export function subscribeEvents(userId: string, send: (event: RelayEvent) => void) {
  const subscriberId = nextSubscriber++;
  subscribers.set(subscriberId, { userId, send });
  return () => subscribers.delete(subscriberId);
}
