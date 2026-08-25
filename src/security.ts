import { Elysia } from "elysia";
import { PUBLIC_URL } from "./config";
import { sha } from "./db";

type Bucket = { count: number; resetAt: number };
const buckets = new Map<string, Bucket>();

function requestKey(request: Request) {
  const bearer = request.headers.get("authorization")?.match(/^Bearer\s+(.+)$/i)?.[1];
  if (bearer) return `bearer:${sha(bearer)}`;
  const apiKey = request.headers.get("x-rc-key-id");
  if (apiKey) return `api-key:${apiKey}`;
  const device = request.headers.get("x-rc-device");
  if (device) return `device:${device}`;
  const session = request.headers.get("cookie")?.match(/(?:^|;\s*)rc_session=([^;]+)/)?.[1];
  if (session) return `session:${sha(decodeURIComponent(session))}`;
  const forwarded = request.headers.get("x-forwarded-for")?.split(",").map(value => value.trim()).filter(Boolean);
  const ip = request.headers.get("cf-connecting-ip")
    || request.headers.get("x-real-ip")
    || forwarded?.at(-1)
    || "unknown";
  return `ip:${ip}`;
}

function policy(request: Request) {
  const url = new URL(request.url), path = url.pathname;
  if (path.startsWith("/assets/") || path.startsWith("/downloads/") || path === "/install.sh") return null;
  if (path === "/api/v1/agent/challenge") return { name: "agent-challenge", limit: 60, windowMs: 60_000 };
  if (path === "/api/v1/agent/enroll") return { name: "agent-enroll", limit: 20, windowMs: 10 * 60_000 };
  if (path.startsWith("/api/v1/auth/")) return { name: `auth:${path.split("/").slice(4, 6).join(":")}`, limit: 30, windowMs: 5 * 60_000 };
  if (path === "/api/v1/agent/ws" || path === "/api/v1/ws") return { name: "ws-connect", limit: 30, windowMs: 60_000 };
  if (path.startsWith("/api/v1/") || request.method !== "GET") return { name: "api", limit: 600, windowMs: 60_000 };
  return null;
}

function checkRateLimit(request: Request) {
  const selected = policy(request);
  if (!selected) return null;
  const now = Date.now(), key = `${selected.name}:${requestKey(request)}`;
  let bucket = buckets.get(key);
  if (!bucket && buckets.size >= 10_000) return 60;
  if (!bucket || bucket.resetAt <= now) bucket = { count: 0, resetAt: now + selected.windowMs };
  bucket.count += 1;
  buckets.set(key, bucket);
  if (bucket.count <= selected.limit) return null;
  return Math.max(1, Math.ceil((bucket.resetAt - now) / 1000));
}

function headers() {
  const secure = PUBLIC_URL.startsWith("https://");
  const websocket = PUBLIC_URL.replace(/^http/, "ws");
  const csp = [
    "default-src 'self'", "base-uri 'none'", "object-src 'none'", "frame-ancestors 'none'",
    "form-action 'self'", "script-src 'self' https://assets.ohrats.party",
    "style-src 'self' https://assets.ohrats.party", "img-src 'self' https://assets.ohrats.party data:",
    "font-src 'self' https://assets.ohrats.party data:", `connect-src 'self' ${websocket}`,
    ...(secure ? ["upgrade-insecure-requests"] : []),
  ].join("; ");
  return {
    "content-security-policy": csp,
    "referrer-policy": "no-referrer",
    "x-content-type-options": "nosniff",
    "x-frame-options": "DENY",
    "permissions-policy": "camera=(), microphone=(), geolocation=(), payment=(), usb=()",
    "cross-origin-opener-policy": "same-origin",
    ...(secure ? { "strict-transport-security": "max-age=31536000" } : {}),
  };
}

setInterval(() => {
  const now = Date.now();
  for (const [key, bucket] of buckets) if (bucket.resetAt <= now) buckets.delete(key);
}, 60_000).unref();

export const security = new Elysia({ name: "rc.security" })
  .onRequest(({ request, set }) => {
    Object.assign(set.headers, headers());
    const retryAfter = checkRateLimit(request);
    if (!retryAfter) return;
    set.status = 429;
    set.headers["retry-after"] = String(retryAfter);
    set.headers["cache-control"] = "no-store";
    return { error: "rate limit exceeded" };
  })
  .as("global");
