import { PUBLIC_URL, SETUP_COOKIE_TTL } from "./config";
import { cookieMaxAge, WEB_DEFAULT_LIFETIME } from "./lifetimes";

export function json(data: unknown, status = 200, headers: HeadersInit = {}) {
  return Response.json(data, { status, headers: { "cache-control": "no-store", ...headers } });
}

export function fail(message: string, status = 400) { return json({ error: message }, status); }

export async function body(req: Request) {
  if (Number(req.headers.get("content-length") || 0) > 1024 * 1024) throw new Error("request too large");
  return await req.json();
}

export function cookie(req: Request, name: string) {
  for (const part of (req.headers.get("cookie") || "").split(";")) {
    const [key, ...rest] = part.trim().split("=");
    if (key === name) return decodeURIComponent(rest.join("="));
  }
  return "";
}

function secureFlag() { return PUBLIC_URL.startsWith("https://") ? "; Secure" : ""; }
export function sessionCookie(token: string, maxAge = cookieMaxAge(WEB_DEFAULT_LIFETIME)) {
  return `rc_session=${encodeURIComponent(token)}; Path=/; HttpOnly; SameSite=Lax; Max-Age=${maxAge}${secureFlag()}`;
}
export function setupCookie(token: string) {
  return `rc_setup=${encodeURIComponent(token)}; Path=/; HttpOnly; SameSite=Strict; Max-Age=${SETUP_COOKIE_TTL}${secureFlag()}`;
}

export function checkOrigin(req: Request) {
  if (["GET", "HEAD", "OPTIONS"].includes(req.method) || req.headers.has("authorization")) return true;
  const origin = req.headers.get("origin");
  if (origin === "null") {
    return req.headers.get("sec-fetch-site") === "same-origin"
      && req.headers.get("sec-fetch-mode") === "navigate"
      && req.headers.get("sec-fetch-dest") === "document";
  }
  if (origin) {
    try {
      const normalized = new URL(origin).origin;
      return normalized === new URL(req.url).origin || normalized === new URL(PUBLIC_URL).origin;
    } catch { return false; }
  }
  const cookies = req.headers.get("cookie") || "";
  if (/(?:^|;\s*)(?:rc_session|rc_setup)=/.test(cookies)) return req.headers.get("sec-fetch-site") === "same-origin";
  return true;
}
