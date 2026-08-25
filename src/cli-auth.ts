import { PUBLIC_URL } from "./config";
import type { User } from "./core";
import { id, now, opaque, q, sha } from "./db";
import { base64urlToBytes } from "./encoding";
import { HttpError } from "./errors";
import { CLI_DEFAULT_LIFETIME, authLifetime, expiresAt, type AuthLifetime } from "./lifetimes";

const REQUEST_TTL = 10 * 60 * 1000;

export function startCliAuthorization(clientIdValue: unknown, publicKeyValue: unknown, lifetimeValue?: unknown) {
  q("DELETE FROM cli_authorizations WHERE expires_at<=? OR exchanged_at IS NOT NULL").run(now());
  q("DELETE FROM cli_sessions WHERE expires_at>0 AND expires_at<=?").run(now());
  const clientId = String(clientIdValue || "").trim(), signingPublicKey = String(publicKeyValue || "").trim();
  const lifetime = authLifetime(lifetimeValue, CLI_DEFAULT_LIFETIME);
  try { if (!clientId || clientId.length > 100 || base64urlToBytes(signingPublicKey).length !== 32) throw new Error(); }
  catch { throw new HttpError(400, "invalid CLI control key"); }
  const requestId = id(), deviceCode = opaque("cli_device"), userCode = opaque("cli_user"), t = now();
  q(`INSERT INTO cli_authorizations(id,device_code_hash,user_code_hash,client_id,signing_public_key,session_lifetime,created_at,expires_at)
    VALUES(?,?,?,?,?,?,?,?)`).run(requestId, sha(deviceCode), sha(userCode), clientId, signingPublicKey, lifetime, t, t + REQUEST_TTL);
  return {
    requestId, deviceCode, expiresAt: t + REQUEST_TTL, interval: 2,
    verificationUrl: `${PUBLIC_URL}/cli/login?code=${encodeURIComponent(userCode)}`,
  };
}

export function cliAuthorizationPreview(value: unknown) {
  const code = String(value || "").trim();
  if (!code) return null;
  return q<{ id: string; client_id: string; signing_public_key: string; session_lifetime: AuthLifetime; approved_at: number | null; exchanged_at: number | null }>(`SELECT id,client_id,signing_public_key,session_lifetime,approved_at,exchanged_at
    FROM cli_authorizations WHERE user_code_hash=? AND expires_at>? AND exchanged_at IS NULL`).get(sha(code), now()) || null;
}

export function approveCliAuthorization(user: User, value: unknown) {
  const code = String(value || "").trim(), row = cliAuthorizationPreview(code);
  if (!row || row.exchanged_at) throw new HttpError(410, "CLI authorization expired");
  if (row.approved_at) throw new HttpError(409, "CLI authorization already approved");
  q("UPDATE cli_authorizations SET user_id=?,approved_at=? WHERE id=?").run(user.id, now(), row.id);
}

export function exchangeCliAuthorization(requestIdValue: unknown, deviceCodeValue: unknown) {
  const requestId = String(requestIdValue || "").trim(), deviceCode = String(deviceCodeValue || "").trim();
  const row = q<{ id: string; user_id: string | null; session_lifetime: AuthLifetime; approved_at: number | null; exchanged_at: number | null }>(`SELECT id,user_id,session_lifetime,approved_at,exchanged_at
    FROM cli_authorizations WHERE id=? AND device_code_hash=? AND expires_at>?`).get(requestId, sha(deviceCode), now());
  if (!row || row.exchanged_at) throw new HttpError(410, "CLI authorization expired");
  if (!row.approved_at || !row.user_id) return { pending: true as const };
  const token = opaque("rc_cli"), t = now(), expiration = expiresAt(authLifetime(row.session_lifetime, CLI_DEFAULT_LIFETIME), t);
  q("INSERT INTO cli_sessions(token_hash,user_id,created_at,expires_at) VALUES(?,?,?,?)")
    .run(sha(token), row.user_id, t, expiration);
  q("UPDATE cli_authorizations SET exchanged_at=? WHERE id=?").run(t, row.id);
  const user = q<User>("SELECT id,name FROM users WHERE id=?").get(row.user_id);
  return { pending: false as const, token, expiresAt: expiration, user };
}

export function cliAuthorizationControl(value: unknown) {
  const row = cliAuthorizationPreview(value);
  return row ? { clientId: row.client_id, signingPublicKey: row.signing_public_key } : null;
}

export function cliTokenUser(token: string): User | null {
  const tokenHash = sha(token);
  const row = q<User>(`SELECT u.id,u.name FROM cli_sessions s JOIN users u ON u.id=s.user_id
    WHERE s.token_hash=? AND (s.expires_at=0 OR s.expires_at>?)`).get(tokenHash, now());
  if (!row) return null;
  q("UPDATE cli_sessions SET last_used=? WHERE token_hash=?").run(now(), tokenHash);
  return row;
}

export function revokeCliToken(token: string) {
  return q("DELETE FROM cli_sessions WHERE token_hash=?").run(sha(token)).changes > 0;
}
