import { api } from './api.js';

function b64urlToBytes(value) {
  const base64 = value.replace(/-/g, '+').replace(/_/g, '/') + '='.repeat((4 - value.length % 4) % 4);
  return Uint8Array.from(atob(base64), char => char.charCodeAt(0));
}

function bytesToB64url(value) {
  const bytes = new Uint8Array(value);
  let binary = '';
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '');
}

function creationOptions(options) {
  if (PublicKeyCredential.parseCreationOptionsFromJSON) return PublicKeyCredential.parseCreationOptionsFromJSON(options);
  return {
    ...options,
    challenge: b64urlToBytes(options.challenge),
    user: { ...options.user, id: b64urlToBytes(options.user.id) },
    excludeCredentials: (options.excludeCredentials || []).map(item => ({ ...item, id: b64urlToBytes(item.id) })),
  };
}

function requestOptions(options) {
  if (PublicKeyCredential.parseRequestOptionsFromJSON) return PublicKeyCredential.parseRequestOptionsFromJSON(options);
  return {
    ...options,
    challenge: b64urlToBytes(options.challenge),
    allowCredentials: options.allowCredentials?.map(item => ({ ...item, id: b64urlToBytes(item.id) })),
  };
}

function credentialJSON(credential) {
  if (credential.toJSON) return credential.toJSON();
  const response = credential.response;
  const output = {
    id: credential.id,
    rawId: bytesToB64url(credential.rawId),
    type: credential.type,
    authenticatorAttachment: credential.authenticatorAttachment,
    clientExtensionResults: credential.getClientExtensionResults(),
  };
  if ('attestationObject' in response) {
    output.response = {
      clientDataJSON: bytesToB64url(response.clientDataJSON),
      attestationObject: bytesToB64url(response.attestationObject),
      transports: response.getTransports?.() || [],
    };
  } else {
    output.response = {
      clientDataJSON: bytesToB64url(response.clientDataJSON),
      authenticatorData: bytesToB64url(response.authenticatorData),
      signature: bytesToB64url(response.signature),
      ...(response.userHandle ? { userHandle: bytesToB64url(response.userHandle) } : {}),
    };
  }
  return output;
}

function requirePasskeys() {
  if (!window.PublicKeyCredential || !navigator.credentials) throw new Error('Passkeys are not supported in this browser.');
}

export async function createPasskey(path, verifyPath, data) {
  requirePasskeys();
  const start = await api(path, { method: 'POST', body: JSON.stringify(data) });
  const credential = await navigator.credentials.create({ publicKey: creationOptions(start.options) });
  if (!credential) throw new Error('Passkey creation was cancelled.');
  return api(verifyPath, {
    method: 'POST',
    body: JSON.stringify({ ceremonyId: start.ceremonyId, response: credentialJSON(credential) }),
  });
}

export async function authenticatePasskey() {
  requirePasskeys();
  const start = await api('/api/v1/auth/login/options', { method: 'POST', body: '{}' });
  const credential = await navigator.credentials.get({ publicKey: requestOptions(start.options) });
  if (!credential) throw new Error('Sign in was cancelled.');
  return api('/api/v1/auth/login/verify', {
    method: 'POST',
    body: JSON.stringify({ ceremonyId: start.ceremonyId, response: credentialJSON(credential) }),
  });
}
