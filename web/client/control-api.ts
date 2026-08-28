import { api } from "./http";

export function controlChallenge(deviceId: string) {
  return api<{ challenge: string }>("/api/v1/control/challenge", { method: "POST", body: JSON.stringify({ deviceId }) });
}

export function controlOpen(input: { deviceId: string; challenge: string; clientId: string; publicKey: string; signature: string }) {
  return api<{ sessionId: string; transportPublicKey: string; ephemeralPublicKey: string; signature: string; iceServers?: RTCIceServer[] }>(
    "/api/v1/control/open", { method: "POST", body: JSON.stringify(input) });
}

export function controlWebRTC(
  sessionId: string,
  deviceId: string,
  sdp: string,
  mode: "host" | "stun" | "relay" = "stun",
) {
  return api<{ sdp: string }>(`/api/v1/control/${encodeURIComponent(sessionId)}/webrtc`, {
    method: "POST", body: JSON.stringify({ deviceId, sdp, mode }),
  });
}

export function closeControlSession(sessionId: string) {
  void fetch(`/api/v1/control/${encodeURIComponent(sessionId)}`, {
    method: "DELETE", credentials: "same-origin", keepalive: true,
    headers: { accept: "application/json" },
  }).catch(() => {});
}
