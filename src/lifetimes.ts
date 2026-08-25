import { HttpError } from "./errors";

export const AUTH_LIFETIMES = ["1h", "1d", "7d", "30d", "90d", "180d", "1y", "never"] as const;
export type AuthLifetime = typeof AUTH_LIFETIMES[number];
export type WebLifetime = Exclude<AuthLifetime, "never">;

export const WEB_DEFAULT_LIFETIME: WebLifetime = "30d";
export const CLI_DEFAULT_LIFETIME: AuthLifetime = "never";
export const MCP_DEFAULT_LIFETIME: AuthLifetime = "never";
export const API_DEFAULT_LIFETIME: AuthLifetime = "never";
export const CONTROL_DEFAULT_LIFETIME: AuthLifetime = "30d";

const durationMs: Record<WebLifetime, number> = {
  "1h": 60 * 60_000,
  "1d": 24 * 60 * 60_000,
  "7d": 7 * 24 * 60 * 60_000,
  "30d": 30 * 24 * 60 * 60_000,
  "90d": 90 * 24 * 60 * 60_000,
  "180d": 180 * 24 * 60 * 60_000,
  "1y": 365 * 24 * 60 * 60_000,
};

export const AUTH_LIFETIME_OPTIONS: Array<{ value: AuthLifetime; label: string }> = [
  { value: "1h", label: "1 hour" }, { value: "1d", label: "1 day" }, { value: "7d", label: "7 days" },
  { value: "30d", label: "30 days" }, { value: "90d", label: "90 days" }, { value: "180d", label: "180 days" },
  { value: "1y", label: "1 year" }, { value: "never", label: "Until revoked" },
];

export function authLifetime(value: unknown, fallback: AuthLifetime, allowNever = true): AuthLifetime {
  const selected = String(value || fallback) as AuthLifetime;
  if (!AUTH_LIFETIMES.includes(selected) || (!allowNever && selected === "never")) throw new HttpError(400, "invalid authorization lifetime");
  return selected;
}

export function expiresAt(lifetime: AuthLifetime, issuedAt = Date.now()) {
  return lifetime === "never" ? 0 : issuedAt + durationMs[lifetime];
}

export function expiryActive(value: number, at = Date.now()) { return value === 0 || value > at; }
export function cookieMaxAge(lifetime: WebLifetime) { return Math.floor(durationMs[lifetime] / 1000); }
export function lifetimeLabel(value: AuthLifetime) { return AUTH_LIFETIME_OPTIONS.find(item => item.value === value)?.label || value; }
export const MAX_FINITE_AUTH_LIFETIME_MS = 366 * 24 * 60 * 60_000;
