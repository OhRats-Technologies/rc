export type AuthorityStatus = {
  hash: string;
  devices: number;
  synced: number;
  deviceStates: Array<{ id: string; online: boolean; synced: boolean }>;
};

export function authorityProgress(state: AuthorityStatus, selected?: string[]) {
  if (!selected) return { devices: state.devices, synced: state.synced };
  const devices = state.deviceStates.filter(device => selected.includes(device.id));
  if (!devices.length) throw new Error("Selected machines are no longer available in this workspace.");
  if (devices.some(device => !device.online && !device.synced)) {
    throw new Error("A selected machine is offline. Bring it online or deselect it before authorizing Terminal.");
  }
  return { devices: devices.length, synced: devices.filter(device => device.synced).length };
}
