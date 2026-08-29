function qs(selector) {
  const element = document.querySelector(selector);
  if (!element) throw new Error(`Missing element: ${selector}`);
  return element;
}
function b64urlToBytes(value) {
  const base64 = value.replace(/-/g, "+").replace(/_/g, "/") + "=".repeat((4 - value.length % 4) % 4);
  return Uint8Array.from(atob(base64), char => char.charCodeAt(0));
}
function bytesToB64url(value) {
  let binary = "";
  for (const byte of new Uint8Array(value)) binary += String.fromCharCode(byte);
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/g, "");
}
function creationOptions(options) {
  const parser = PublicKeyCredential.parseCreationOptionsFromJSON;
  if (parser) return parser(options);
  return { ...options, challenge: b64urlToBytes(options.challenge),
    user: { ...options.user, id: b64urlToBytes(options.user.id) },
    excludeCredentials: (options.excludeCredentials || []).map(item => ({ ...item, id: b64urlToBytes(item.id) })) };
}
function requestOptions(options) {
  const parser = PublicKeyCredential.parseRequestOptionsFromJSON;
  if (parser) return parser(options);
  return { ...options, challenge: b64urlToBytes(options.challenge),
    allowCredentials: (options.allowCredentials || []).map(item => ({ ...item, id: b64urlToBytes(item.id) })) };
}
function credentialJSON(credential) {
  if (credential.toJSON) return credential.toJSON();
  const response = credential.response;
  const output = { id: credential.id, rawId: bytesToB64url(credential.rawId), type: credential.type,
    authenticatorAttachment: credential.authenticatorAttachment,
    clientExtensionResults: credential.getClientExtensionResults() };
  if (response instanceof AuthenticatorAttestationResponse) {
    output.response = { clientDataJSON: bytesToB64url(response.clientDataJSON),
      attestationObject: bytesToB64url(response.attestationObject), transports: response.getTransports?.() || [] };
  } else {
    output.response = { clientDataJSON: bytesToB64url(response.clientDataJSON),
      authenticatorData: bytesToB64url(response.authenticatorData), signature: bytesToB64url(response.signature),
      ...(response.userHandle ? { userHandle: bytesToB64url(response.userHandle) } : {}) };
  }
  return output;
}
async function api(path, options = {}) {
  const response = await fetch(path, { ...options, headers: { "content-type": "application/json", ...options.headers } });
  let body = {};
  try { body = await response.json(); } catch {}
  if (!response.ok) throw new Error(body.error || response.statusText);
  return body;
}
function requirePasskeys() {
  if (!window.PublicKeyCredential || !navigator.credentials) throw new Error("Passkeys are not supported in this browser.");
}
async function createPasskey(path, verifyPath, data) {
  requirePasskeys();
  const start = await api(path, { method: "POST", body: JSON.stringify(data) });
  const credential = await navigator.credentials.create({ publicKey: creationOptions(start.options) });
  if (!credential) throw new Error("Passkey creation was cancelled.");
  return api(verifyPath, { method: "POST", body: JSON.stringify({ ceremonyId: start.ceremonyId, response: credentialJSON(credential) }) });
}
async function authenticatePasskey(lifetime) {
  requirePasskeys();
  const start = await api("/api/v1/auth/login/options", { method: "POST", body: "{}" });
  const credential = await navigator.credentials.get({ publicKey: requestOptions(start.options) });
  if (!credential) throw new Error("Passkey request was cancelled.");
  return api("/api/v1/auth/login/verify", { method: "POST", body: JSON.stringify({ ceremonyId: start.ceremonyId, response: credentialJSON(credential), lifetime }) });
}
function message(error) {
  if (error instanceof DOMException && error.name === "NotAllowedError") return "Passkey request cancelled.";
  return error instanceof Error ? error.message : "Authentication failed.";
}
function errorOut(error) { qs("#auth-error").textContent = message(error); }
function destination() { return qs(".auth-content").dataset.authNext || `${location.origin}/devices`; }
document.querySelector("#setup-form")?.addEventListener("submit", async event => {
  event.preventDefault();
  try {
    const name = String(new FormData(event.currentTarget).get("name") || "");
    await createPasskey("/api/v1/auth/setup/options", "/api/v1/auth/setup/verify", { name });
    location.href = "/devices";
  } catch (error) { errorOut(error); }
});
document.querySelector("#login-form")?.addEventListener("submit", async event => {
  event.preventDefault();
  try {
    const lifetime = String(new FormData(event.currentTarget).get("lifetime") || "30d");
    await authenticatePasskey(lifetime);
    location.href = destination();
  } catch (error) { errorOut(error); }
});
