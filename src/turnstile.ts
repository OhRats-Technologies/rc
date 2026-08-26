import { PUBLIC_SIGNUP_CONFIGURED, RP_ID, TURNSTILE_SECRET_KEY } from "./config";
import { HttpError } from "./errors";

type SiteverifyResult = { success?: boolean; hostname?: string; action?: string };

export async function verifySignupTurnstile(tokenValue: unknown, fetcher: typeof fetch = fetch) {
  if (!PUBLIC_SIGNUP_CONFIGURED) throw new HttpError(404, "public signup is unavailable");
  const token = String(tokenValue || "");
  if (!token || token.length > 2048) throw new HttpError(403, "human verification required");
  let result: SiteverifyResult;
  try {
    const response = await fetcher("https://challenges.cloudflare.com/turnstile/v0/siteverify", {
      method: "POST",
      headers: { "content-type": "application/x-www-form-urlencoded" },
      body: new URLSearchParams({ secret: TURNSTILE_SECRET_KEY, response: token }),
      signal: AbortSignal.timeout(5_000),
    });
    if (!response.ok) throw new Error("Siteverify unavailable");
    result = await response.json() as SiteverifyResult;
  } catch {
    throw new HttpError(503, "human verification unavailable");
  }
  if (!result.success || result.hostname !== RP_ID || result.action !== "signup") {
    throw new HttpError(403, "human verification failed");
  }
}
