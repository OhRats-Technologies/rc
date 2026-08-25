import { api } from "./http";

function b64urlToBytes(value: string) {
  const base64 = value.replace(/-/g, "+").replace(/_/g, "/") + "=".repeat((4 - value.length % 4) % 4);
  return Uint8Array.from(atob(base64), char => char.charCodeAt(0));
}
function bytesToB64url(value: ArrayBuffer) {
  let binary = ""; for (const byte of new Uint8Array(value)) binary += String.fromCharCode(byte);
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/g, "");
}
function creationOptions(options: any): PublicKeyCredentialCreationOptions {
  const parser = (PublicKeyCredential as any).parseCreationOptionsFromJSON;
  return parser ? parser(options) : { ...options, challenge: b64urlToBytes(options.challenge), user: { ...options.user, id: b64urlToBytes(options.user.id) },
    excludeCredentials: (options.excludeCredentials || []).map((item: any) => ({ ...item, id: b64urlToBytes(item.id) })) };
}
export function requestOptions(options: any): PublicKeyCredentialRequestOptions {
  const parser = (PublicKeyCredential as any).parseRequestOptionsFromJSON;
  return parser ? parser(options) : { ...options, challenge: b64urlToBytes(options.challenge),
    allowCredentials: options.allowCredentials?.map((item: any) => ({ ...item, id: b64urlToBytes(item.id) })) };
}
export function credentialJSON(credential: PublicKeyCredential) {
  const native = (credential as any).toJSON; if (native) return native.call(credential);
  const response = credential.response, output: any = { id: credential.id, rawId: bytesToB64url(credential.rawId), type: credential.type,
    authenticatorAttachment: credential.authenticatorAttachment, clientExtensionResults: credential.getClientExtensionResults() };
  if (response instanceof AuthenticatorAttestationResponse) output.response = { clientDataJSON: bytesToB64url(response.clientDataJSON), attestationObject: bytesToB64url(response.attestationObject), transports: response.getTransports?.() || [] };
  else if (response instanceof AuthenticatorAssertionResponse) output.response = { clientDataJSON: bytesToB64url(response.clientDataJSON), authenticatorData: bytesToB64url(response.authenticatorData), signature: bytesToB64url(response.signature), ...(response.userHandle ? { userHandle: bytesToB64url(response.userHandle) } : {}) };
  return output;
}
function requirePasskeys() { if (!window.PublicKeyCredential || !navigator.credentials) throw new Error("Passkeys are not supported in this browser."); }

export async function passkeyAssertion(options: any) {
  requirePasskeys();
  const credential = await navigator.credentials.get({ publicKey: requestOptions(options) }) as PublicKeyCredential | null;
  if (!credential) throw new Error("Passkey request was cancelled.");
  return credentialJSON(credential);
}

export async function createPasskey(path: string, verifyPath: string, data: Record<string, unknown>, headers: Record<string, string> = {}) {
  requirePasskeys(); const start = await api<{ ceremonyId: string; options: any }>(path, { method: "POST", headers, body: JSON.stringify(data) });
  const credential = await navigator.credentials.create({ publicKey: creationOptions(start.options) }) as PublicKeyCredential | null;
  if (!credential) throw new Error("Passkey creation was cancelled.");
  return api(verifyPath, { method: "POST", body: JSON.stringify({ ceremonyId: start.ceremonyId, response: credentialJSON(credential) }) });
}
export async function authenticatePasskey() {
  requirePasskeys(); const start = await api<{ ceremonyId: string; options: any }>("/api/v1/auth/login/options", { method: "POST", body: "{}" });
  return api("/api/v1/auth/login/verify", { method: "POST", body: JSON.stringify({ ceremonyId: start.ceremonyId, response: await passkeyAssertion(start.options) }) });
}
