import { expect, test } from "bun:test";
import { onlineNodesPending, syncSettled, type AuthorityStatus } from "./authority-status";

const state = (deviceStates: AuthorityStatus["deviceStates"]): AuthorityStatus => ({
  hash: "hash",
  devices: deviceStates.length,
  synced: deviceStates.filter(device => device.synced).length,
  deviceStates,
});

test("offline Nodes do not block a sync from settling", () => {
  const status = state([
    { id: "online-current", name: "online-current", online: true, synced: true },
    { id: "offline-stale", name: "offline-stale", online: false, synced: false },
  ]);
  expect(onlineNodesPending(status)).toBe(0);
  expect(syncSettled(status)).toBe(true);
});

test("an online Node that has not accepted the lock keeps the sync pending", () => {
  const status = state([
    { id: "online-stale", name: "online-stale", online: true, synced: false },
    { id: "offline-stale", name: "offline-stale", online: false, synced: false },
  ]);
  expect(onlineNodesPending(status)).toBe(1);
  expect(syncSettled(status)).toBe(false);
});

test("a selected sync (Terminal) still requires every selected Node", () => {
  const status = state([
    { id: "a", name: "a", online: true, synced: true },
    { id: "b", name: "b", online: true, synced: false },
  ]);
  expect(syncSettled(status, ["a"])).toBe(true);
  expect(syncSettled(status, ["a", "b"])).toBe(false);
});
