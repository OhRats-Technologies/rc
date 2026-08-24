import { PUBLIC_URL, SESSION_TTL, SETUP_COOKIE_TTL } from "./config";

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
export function sessionCookie(token: string, maxAge = Math.floor(SESSION_TTL / 1000)) {
  return `relay_session=${encodeURIComponent(token)}; Path=/; HttpOnly; SameSite=Lax; Max-Age=${maxAge}${secureFlag()}`;
}
export function setupCookie(token: string) {
  return `relay_setup=${encodeURIComponent(token)}; Path=/; HttpOnly; SameSite=Strict; Max-Age=${SETUP_COOKIE_TTL}${secureFlag()}`;
}

export function checkOrigin(req: Request) {
  if (["GET", "HEAD", "OPTIONS"].includes(req.method) || req.headers.has("authorization")) return true;
  const origin = req.headers.get("origin");
  return !origin || origin === new URL(req.url).origin || origin === PUBLIC_URL;
}
