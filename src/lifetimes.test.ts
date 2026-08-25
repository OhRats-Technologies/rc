import { describe, expect, test } from "bun:test";
import {
  API_DEFAULT_LIFETIME, AUTH_LIFETIMES, CLI_DEFAULT_LIFETIME, MCP_DEFAULT_LIFETIME, WEB_DEFAULT_LIFETIME,
  authLifetime, cookieMaxAge, expiresAt, expiryActive,
} from "./lifetimes";

describe("authorization lifetimes", () => {
  test("defaults match product policy", () => {
    expect(WEB_DEFAULT_LIFETIME).toBe("30d");
    expect(CLI_DEFAULT_LIFETIME).toBe("never");
    expect(MCP_DEFAULT_LIFETIME).toBe("never");
    expect(API_DEFAULT_LIFETIME).toBe("never");
  });

  test("supports the advertised presets and explicit until-revoked expiry", () => {
    expect(AUTH_LIFETIMES).toEqual(["1h", "1d", "7d", "30d", "90d", "180d", "1y", "never"]);
    expect(expiresAt("never", 123)).toBe(0);
    expect(expiryActive(0, Number.MAX_SAFE_INTEGER)).toBe(true);
    expect(expiresAt("1h", 1000)).toBe(1000 + 60 * 60_000);
    expect(cookieMaxAge("30d")).toBe(30 * 24 * 60 * 60);
  });

  test("web sessions reject until-revoked while durable credentials may use it", () => {
    expect(authLifetime("never", "30d")).toBe("never");
    expect(() => authLifetime("never", "30d", false)).toThrow("invalid authorization lifetime");
    expect(() => authLifetime("forever", "30d")).toThrow("invalid authorization lifetime");
  });
});
