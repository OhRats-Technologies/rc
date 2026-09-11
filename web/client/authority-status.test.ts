import { expect, test } from "bun:test";
import { authorityProgress, type AuthorityStatus } from "./authority-status";

const state: AuthorityStatus = {
  hash: "snapshot", devices: 2, synced: 1,
  deviceStates: [
    { id: "pc", online: true, synced: true },
    { id: "mac", online: false, synced: false },
  ],
};

test("an unselected offline machine does not block MCP authority sync", () => {
  expect(authorityProgress(state, ["pc"])).toEqual({ devices: 1, synced: 1 });
  expect(authorityProgress(state)).toEqual({ devices: 2, synced: 1 });
});

test("a selected unsynced machine must accept the lock", () => {
  expect(() => authorityProgress(state, ["pc", "mac"])).toThrow("offline");
  const pending = { ...state, deviceStates: [{ id: "pc", online: true, synced: false }] };
  expect(authorityProgress(pending, ["pc"])).toEqual({ devices: 1, synced: 0 });
  expect(() => authorityProgress(state, ["removed"])).toThrow("no longer available");
});
