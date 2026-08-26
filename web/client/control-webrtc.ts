import { request } from "./socket";
import type { ControlTransport } from "./control-transport";

type Frame = { type: "control.frame"; sessionId: string; sequence: number; ciphertext: string };

function waitForGathering(peer: RTCPeerConnection) {
  if (peer.iceGatheringState === "complete") return Promise.resolve();
  return new Promise<void>((resolve, reject) => {
    const timer = window.setTimeout(() => { cleanup(); reject(new Error("WebRTC ICE gathering timed out")); }, 5000);
    const change = () => { if (peer.iceGatheringState === "complete") { cleanup(); resolve(); } };
    const cleanup = () => { clearTimeout(timer); peer.removeEventListener("icegatheringstatechange", change); };
    peer.addEventListener("icegatheringstatechange", change);
  });
}

function waitForOpen(channel: RTCDataChannel) {
  if (channel.readyState === "open") return Promise.resolve();
  return new Promise<void>((resolve, reject) => {
    const timer = window.setTimeout(() => { cleanup(); reject(new Error("WebRTC connection timed out")); }, 7000);
    const open = () => { cleanup(); resolve(); }, error = () => { cleanup(); reject(new Error("WebRTC connection failed")); };
    const cleanup = () => { clearTimeout(timer); channel.removeEventListener("open", open); channel.removeEventListener("error", error); };
    channel.addEventListener("open", open, { once: true }); channel.addEventListener("error", error, { once: true });
  });
}

export async function openWebRTCControlTransport(deviceId: string, sessionId: string, iceServers: RTCIceServer[], fallback: ControlTransport): Promise<ControlTransport | null> {
  if (typeof RTCPeerConnection === "undefined") return null;
  const peer = new RTCPeerConnection({ iceServers }), channel = peer.createDataChannel("rc-control", { ordered: true });
  const frameListeners = new Set<(sequence: number, ciphertext: string) => void>();
  let direct = true, closing = false;
  const fallbackFrame = fallback.onFrame((sequence, ciphertext) => {
    if (closing) return;
    direct = false; peer.close();
    for (const listener of frameListeners) listener(sequence, ciphertext);
  });
  channel.addEventListener("message", event => {
    if (!direct || closing) return;
    if (typeof event.data !== "string" || event.data.length > 2_000_000) { direct = false; peer.close(); return; }
    let frame: Frame; try { frame = JSON.parse(String(event.data)) as Frame; } catch { return; }
    if (frame.type !== "control.frame" || frame.sessionId !== sessionId) return;
    for (const listener of frameListeners) listener(Number(frame.sequence), String(frame.ciphertext || ""));
  });
  channel.addEventListener("close", () => { if (!closing) direct = false; });
  try {
    await peer.setLocalDescription(await peer.createOffer()); await waitForGathering(peer);
    const sdp = peer.localDescription?.sdp; if (!sdp) throw new Error("WebRTC offer unavailable");
    const answer = await request<{ sdp: string }>({ type: "control.webrtc", deviceId, sessionId, sdp });
    await peer.setRemoteDescription({ type: "answer", sdp: answer.sdp }); await waitForOpen(channel);
  } catch {
    closing = true; fallbackFrame(); peer.close(); return null;
  }
  return {
    send(sequence, ciphertext) {
      if (direct && channel.readyState === "open" && channel.bufferedAmount <= 1_048_576) {
        try { channel.send(JSON.stringify({ type: "control.frame", sessionId, sequence, ciphertext } satisfies Frame)); return true; }
        catch { direct = false; peer.close(); }
      }
      direct = false; return fallback.send(sequence, ciphertext);
    },
    onFrame(listener) { frameListeners.add(listener); return () => frameListeners.delete(listener); },
    close() { if (closing) return; closing = true; fallbackFrame(); peer.close(); fallback.close(); },
  };
}
