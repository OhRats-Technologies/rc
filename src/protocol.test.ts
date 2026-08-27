import { expect, test } from "bun:test";
import { Value } from "@sinclair/typebox/value";
import { AgentClientMessageSchema, AgentServerMessageSchema, BrowserCommandSchema, BrowserServerMessageSchema } from "./protocol";

const relayFrame = { type: "control.frame", deviceId: "device", sessionId: "session", sequence: 1, ciphertext: "ciphertext" };

test("control ciphertext is not valid on WebSocket protocols", () => {
  expect(Value.Check(BrowserCommandSchema, relayFrame)).toBe(false);
  expect(Value.Check(BrowserServerMessageSchema, relayFrame)).toBe(false);
  expect(Value.Check(AgentClientMessageSchema, relayFrame)).toBe(false);
  expect(Value.Check(AgentServerMessageSchema, relayFrame)).toBe(false);
});

test("WebRTC transport telemetry distinguishes failure without WSS fallback", () => {
  expect(Value.Check(BrowserCommandSchema, {
    type: "control.transport", deviceId: "device", sessionId: "session", transport: "webrtc", phase: "failed", reason: "DataChannel closed",
  })).toBe(true);
  expect(Value.Check(BrowserCommandSchema, {
    type: "control.transport", deviceId: "device", sessionId: "session", transport: "relay", phase: "fallback",
  })).toBe(false);
});
