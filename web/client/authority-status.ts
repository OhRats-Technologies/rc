export type LockSyncState = "synced" | "unbootstrapped" | "stale";

export type AuthorityStatus = {
  hash: string;
  devices: number;
  synced: number;
  deviceStates: Array<{ id: string; name: string; online: boolean; synced: boolean; lockState?: LockSyncState }>;
};

// AuthoritySyncError carries the server's state so the page can show which Nodes are still pending.
export class AuthoritySyncError extends Error {
  constructor(message: string, readonly state: AuthorityStatus) {
    super(message);
  }
}

export function authorityProgress(state: AuthorityStatus, selected?: string[]) {
  if (!selected) return { devices: state.devices, synced: state.synced };
  const devices = state.deviceStates.filter(device => selected.includes(device.id));
  if (!devices.length) throw new Error("Selected machines are no longer available in this workspace.");
  if (devices.some(device => !device.online && !device.synced)) {
    throw new Error("A selected machine is offline. Bring it online or deselect it before authorizing Terminal.");
  }
  return { devices: devices.length, synced: devices.filter(device => device.synced).length };
}

// Offline Nodes cannot accept an RC Lock now; they apply the stored transition when they reconnect.
export function onlineNodesPending(state: AuthorityStatus) {
  return state.deviceStates.filter(device => device.online && !device.synced).length;
}

// A sync settles when every selected Node accepts it, or when every online Node does without a selection.
export function syncSettled(state: AuthorityStatus, selected?: string[]) {
  if (selected) {
    const progress = authorityProgress(state, selected);
    return !progress.devices || progress.synced === progress.devices;
  }
  return onlineNodesPending(state) === 0;
}
