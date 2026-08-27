import { CF_TURN_API_TOKEN, CF_TURN_TOKEN_ID } from "./config";

export type IceServer = { urls: string[]; username?: string; credential?: string };

const STUN_ONLY: IceServer[] = [{ urls: ["stun:stun.cloudflare.com:3478"] }];
const TURN_TTL_SECONDS = 86_400;
const TURN_REFRESH_MARGIN_MS = 5 * 60 * 1000;
let cached: { servers: IceServer[]; expiresAt: number } | null = null;
let pending: Promise<IceServer[]> | null = null;

function validIceServers(value: unknown): IceServer[] | null {
  if (!Array.isArray(value) || value.length === 0 || value.length > 8) return null;
  const servers: IceServer[] = [];
  for (const item of value) {
    if (!item || typeof item !== "object") return null;
    const raw = item as Record<string, unknown>, urls = raw.urls;
    if (!Array.isArray(urls) || urls.length === 0 || urls.length > 16 ||
      urls.some(url => typeof url !== "string" || url.length < 1 || url.length > 512)) return null;
    const username = raw.username, credential = raw.credential;
    if (username !== undefined && (typeof username !== "string" || username.length > 512)) return null;
    if (credential !== undefined && (typeof credential !== "string" || credential.length > 512)) return null;
    servers.push({
      urls: urls as string[],
      ...(typeof username === "string" && username ? { username } : {}),
      ...(typeof credential === "string" && credential ? { credential } : {}),
    });
  }
  return servers;
}

async function mintCloudflareTurn(): Promise<IceServer[]> {
  const response = await fetch(`https://rtc.live.cloudflare.com/v1/turn/keys/${encodeURIComponent(CF_TURN_TOKEN_ID)}/credentials/generate-ice-servers`, {
    method: "POST", signal: AbortSignal.timeout(5_000),
    headers: { Authorization: `Bearer ${CF_TURN_API_TOKEN}`, "Content-Type": "application/json" },
    body: JSON.stringify({ ttl: TURN_TTL_SECONDS }),
  });
  if (!response.ok) throw new Error(`Cloudflare TURN returned HTTP ${response.status}`);
  const body = await response.json() as { iceServers?: unknown };
  const servers = validIceServers(body.iceServers);
  const hasTurn = servers?.some(server => server.urls.some(url => url.startsWith("turn:") || url.startsWith("turns:")));
  if (!servers || !hasTurn) throw new Error("Cloudflare TURN returned invalid ICE servers");
  cached = { servers, expiresAt: Date.now() + TURN_TTL_SECONDS * 1000 };
  return servers;
}

export async function controlIceServers(): Promise<IceServer[]> {
  if (!CF_TURN_TOKEN_ID || !CF_TURN_API_TOKEN) return STUN_ONLY;
  if (cached && cached.expiresAt - TURN_REFRESH_MARGIN_MS > Date.now()) return cached.servers;
  if (!pending) pending = mintCloudflareTurn().finally(() => { pending = null; });
  try { return await pending; }
  catch (error) {
    if (cached && cached.expiresAt > Date.now()) return cached.servers;
    console.warn("Cloudflare TURN credentials unavailable; using STUN only:",
      error instanceof Error ? error.message : "request failed");
    return STUN_ONLY;
  }
}
