import { fire, request } from "./socket";
import type { ControlTransport } from "./control-transport";

type Frame = { type: "control.frame"; sessionId: string; sequence: number; ciphertext: string };
export type CandidateSummary = { host: number; srflx: number; relay: number; udp: number; tcp: number };
export type ControlTransportStatus = {
  transport: "webrtc" | "relay"; phase?: "connecting" | "connected" | "fallback"; reason?: string;
  iceState?: string; connectionState?: string; localCandidates?: CandidateSummary; remoteCandidates?: CandidateSummary;
  selected?: { localType?: string; remoteType?: string; protocol?: string };
};

type StatusReporter = (status: ControlTransportStatus) => void;

function candidateSummary(sdp = ""): CandidateSummary {
  const result: CandidateSummary = { host: 0, srflx: 0, relay: 0, udp: 0, tcp: 0 };
  for (const line of sdp.split(/\r?\n/)) {
    if (!line.startsWith("a=candidate:")) continue;
    const fields = line.slice(12).trim().split(/\s+/), protocol = String(fields[2] || "").toLowerCase();
    const typeIndex = fields.indexOf("typ"), type = String(typeIndex >= 0 ? fields[typeIndex + 1] || "" : "");
    if (type === "host" || type === "srflx" || type === "relay") result[type]++;
    if (protocol === "udp" || protocol === "tcp") result[protocol]++;
  }
  return result;
}

function waitForGathering(peer: RTCPeerConnection) {
  if (peer.iceGatheringState === "complete") return Promise.resolve();
  return new Promise<void>((resolve, reject) => {
    const timer = window.setTimeout(() => { cleanup(); reject(new Error("browser ICE gathering timed out")); }, 5000);
    const change = () => { if (peer.iceGatheringState === "complete") { cleanup(); resolve(); } };
    const cleanup = () => { clearTimeout(timer); peer.removeEventListener("icegatheringstatechange", change); };
    peer.addEventListener("icegatheringstatechange", change);
  });
}

function waitForOpen(peer: RTCPeerConnection, channel: RTCDataChannel) {
  if (channel.readyState === "open") return Promise.resolve();
  return new Promise<void>((resolve, reject) => {
    const timer = window.setTimeout(() => {
      cleanup(); reject(new Error(`DataChannel timed out (ICE ${peer.iceConnectionState}, peer ${peer.connectionState})`));
    }, 7000);
    const open = () => { cleanup(); resolve(); };
    const error = () => { cleanup(); reject(new Error(`DataChannel failed (ICE ${peer.iceConnectionState}, peer ${peer.connectionState})`)); };
    const cleanup = () => { clearTimeout(timer); channel.removeEventListener("open", open); channel.removeEventListener("error", error); };
    channel.addEventListener("open", open, { once: true }); channel.addEventListener("error", error, { once: true });
  });
}

async function selectedPair(peer: RTCPeerConnection) {
  try {
    const stats = await peer.getStats(); let pair: any = null;
    stats.forEach((value: any) => {
      if (value.type === "candidate-pair" && value.state === "succeeded" && (value.nominated || !pair)) pair = value;
    });
    if (!pair) return undefined;
    const local: any = stats.get(pair.localCandidateId), remote: any = stats.get(pair.remoteCandidateId);
    return {
      localType: String(local?.candidateType || "") || undefined,
      remoteType: String(remote?.candidateType || "") || undefined,
      protocol: String(local?.protocol || remote?.protocol || "") || undefined,
    };
  } catch { return undefined; }
}

export async function openWebRTCControlTransport(deviceId: string, sessionId: string, iceServers: RTCIceServer[], fallback: ControlTransport,
  onStatus: StatusReporter = () => {}): Promise<ControlTransport | null> {
  if (typeof RTCPeerConnection === "undefined") {
    const status: ControlTransportStatus = { transport: "relay", phase: "fallback", reason: "WebRTC unavailable in this browser" };
    onStatus(status); fire({ type: "control.transport", deviceId, sessionId, ...status }); return null;
  }
  const peer = new RTCPeerConnection({ iceServers }), channel = peer.createDataChannel("rc-control", { ordered: true });
  const frameListeners = new Set<(sequence: number, ciphertext: string) => void>();
  let direct = true, closing = false, established = false, localCandidates: CandidateSummary | undefined, remoteCandidates: CandidateSummary | undefined;
  let lastFinal = "";

  const publish = (status: ControlTransportStatus, persist = true) => {
    onStatus(status);
    if (persist) fire({ type: "control.transport", deviceId, sessionId, ...status });
  };
  const relay = (reason: string) => {
    const status: ControlTransportStatus = { transport: "relay", phase: "fallback", reason: reason.slice(0, 200),
      iceState: peer.iceConnectionState, connectionState: peer.connectionState, localCandidates, remoteCandidates };
    if (lastFinal !== `relay:${status.reason}`) { lastFinal = `relay:${status.reason}`; publish(status); }
  };
  publish({ transport: "webrtc", phase: "connecting" }, false);

  const fallbackFrame = fallback.onFrame((sequence, ciphertext) => {
    if (closing) return;
    // Relay is the session's safe transport while ICE is negotiating. An early relay frame must not abort the direct attempt.
    if (established && direct) { direct = false; relay("Node resumed the WebSocket relay"); peer.close(); }
    for (const listener of frameListeners) listener(sequence, ciphertext);
  });
  channel.addEventListener("message", event => {
    if (!direct || closing) return;
    if (typeof event.data !== "string" || event.data.length > 2_000_000) { direct = false; relay("Invalid WebRTC frame"); peer.close(); return; }
    let frame: Frame; try { frame = JSON.parse(String(event.data)) as Frame; } catch { return; }
    if (frame.type !== "control.frame" || frame.sessionId !== sessionId) return;
    for (const listener of frameListeners) listener(Number(frame.sequence), String(frame.ciphertext || ""));
  });
  channel.addEventListener("close", () => {
    if (!closing && established && direct) { direct = false; relay("WebRTC DataChannel closed"); }
  });
  try {
    await peer.setLocalDescription(await peer.createOffer()); await waitForGathering(peer);
    const sdp = peer.localDescription?.sdp; if (!sdp) throw new Error("WebRTC offer unavailable");
    localCandidates = candidateSummary(sdp);
    const answer = await request<{ sdp: string }>({ type: "control.webrtc", deviceId, sessionId, sdp });
    remoteCandidates = candidateSummary(answer.sdp);
    await peer.setRemoteDescription({ type: "answer", sdp: answer.sdp }); await waitForOpen(peer, channel);
    established = true;
    const selected = await selectedPair(peer);
    const status: ControlTransportStatus = { transport: "webrtc", phase: "connected", iceState: peer.iceConnectionState,
      connectionState: peer.connectionState, localCandidates, remoteCandidates, selected };
    lastFinal = "webrtc"; publish(status);
  } catch (error) {
    closing = true;
    const reason = error instanceof Error ? error.message : "WebRTC negotiation failed";
    relay(reason); fallbackFrame(); peer.close(); return null;
  }
  return {
    send(sequence, ciphertext) {
      if (direct && channel.readyState === "open" && channel.bufferedAmount <= 1_048_576) {
        try { channel.send(JSON.stringify({ type: "control.frame", sessionId, sequence, ciphertext } satisfies Frame)); return true; }
        catch { direct = false; relay("WebRTC send failed"); peer.close(); }
      }
      direct = false; return fallback.send(sequence, ciphertext);
    },
    onFrame(listener) { frameListeners.add(listener); return () => frameListeners.delete(listener); },
    close() { if (closing) return; closing = true; fallbackFrame(); peer.close(); fallback.close(); },
  };
}
