import { expect, test } from "bun:test";
import { b64urlToBytes, bytesToB64url, importEd25519VerifyKey } from "./control-client";

test("base64url helpers handle URL-safe alphabet and reject malformed input", () => {
  const bytes = Uint8Array.from([251, 255, 239, 0, 17, 34]);
  const encoded = bytesToB64url(bytes);
  expect(encoded).toContain("-");
  expect(encoded).toContain("_");
  expect(Array.from(b64urlToBytes(encoded))).toEqual(Array.from(bytes));
  expect(() => b64urlToBytes("not+base64url")).toThrow("Invalid base64url data");
});

test("raw base64url Ed25519 public keys import for verification", async () => {
  const pair = await crypto.subtle.generateKey({ name: "Ed25519" }, true, ["sign", "verify"]);
  const raw = await crypto.subtle.exportKey("raw", pair.publicKey);
  const imported = await importEd25519VerifyKey(bytesToB64url(raw));
  const payload = new TextEncoder().encode("rc browser key regression");
  const signature = await crypto.subtle.sign("Ed25519", pair.privateKey, payload);
  expect(await crypto.subtle.verify("Ed25519", imported, signature, payload)).toBe(true);
});
