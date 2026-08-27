import { expect, test } from "bun:test";
import { Value } from "@sinclair/typebox/value";
import { AgentClientMessageSchema, AgentServerMessageSchema } from "./protocol";

const controlFrame = { type: "control.frame", sessionId: "session", sequence: 1, ciphertext: "ciphertext" };

test("control ciphertext is not valid on the agent WebSocket protocol", () => {
  expect(Value.Check(AgentClientMessageSchema, controlFrame)).toBe(false);
  expect(Value.Check(AgentServerMessageSchema, controlFrame)).toBe(false);
});

test("agent protocol reports WebRTC control session closure as metadata", () => {
  expect(Value.Check(AgentClientMessageSchema, { type: "control.closed", sessionId: "session" })).toBe(true);
});
