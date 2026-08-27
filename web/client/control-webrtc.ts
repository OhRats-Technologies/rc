import { fire, request } from "./socket";
import type { ControlTransport } from "./control-transport";

type Frame = { type: "control.frame"; sessionId: string; sequence: number; ciphertext: string };
export type CandidateSummary = { host: number; srflx: number; relay: number; udp: number; tcp: number };
export type ControlTransportStatus = {
  transport: "webrtc"; phase?: "connecting" | "connected" | "failed"; reason?: string;
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

export async function openWebRTCControlTransport(deviceId: string, sessionId: string, iceServers: RTCIceServer[],
  onStatus: StatusReporter = () => {}, attempt = 0): Promise<ControlTransport> {
  if (typeof RTCPeerConnection === "undefined") throw new Error("WebRTC unavailable in this browser");
  const peer = new RTCPeerConnection({ iceServers }), channel = peer.createDataChannel("rc-control", { ordered: true });
  const frameListeners = new Set<(sequence: number, ciphertext: string) => void>();
  let closing = false, established = false, localCandidates: CandidateSummary | undefined, remoteCandidates: CandidateSummary | undefined;

  const publish = (status: ControlTransportStatus, report = true) => {
    onStatus(status);
    if (report) fire({ type: "control.transport", deviceId, sessionId, ...status });
  };
  const fail = (reason: string) => publish({ transport: "webrtc", phase: "failed", reason: reason.slice(0, 200),
    iceState: peer.iceConnectionState, connectionState: peer.connectionState, localCandidates, remoteCandidates });
  publish({ transport: "webrtc", phase: "connecting" }, false);

  channel.addEventListener("message", async event => {
    if (closing) return;
    let text: string;
    if (typeof event.data === "string") text = event.data;
    else if (event.data instanceof ArrayBuffer) text = new TextDecoder().decode(event.data);
    else if (event.data instanceof Blob) text = await event.data.text();
    else { fail("Invalid WebRTC frame"); channel.close(); return; }
    if (text.length > 2_000_000) { fail("Invalid WebRTC frame"); channel.close(); return; }
    let frame: Frame; try { frame = JSON.parse(text) as Frame; } catch { fail("Invalid WebRTC frame"); channel.close(); return; }
    if (frame.type !== "control.frame" || frame.sessionId !== sessionId) return;
    for (const listener of frameListeners) listener(Number(frame.sequence), String(frame.ciphertext || ""));
  });
  channel.addEventListener("close", () => {
    if (!closing && established) { fail("WebRTC DataChannel closed"); fire({ type: "control.close", deviceId, sessionId }); }
  });
  peer.addEventListener("connectionstatechange", () => {
    if (!closing && established && peer.connectionState === "failed") {
      fail("WebRTC peer connection failed"); closing = true; fire({ type: "control.close", deviceId, sessionId }); channel.close(); peer.close();
    }
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
    publish({ transport: "webrtc", phase: "connected", iceState: peer.iceConnectionState,
      connectionState: peer.connectionState, localCandidates, remoteCandidates, selected });
  } catch (error) {
    closing = true;
    const reason = error instanceof Error ? error.message : "WebRTC negotiation failed";
    peer.close();
    if (attempt === 0 && /webrtc|ice|datachannel|timed out|failed/i.test(reason)) {
      onStatus({ transport: "webrtc", phase: "connecting", reason: `Retrying after: ${reason}` });
      await new Promise(resolve => window.setTimeout(resolve, 1200));
      return openWebRTCControlTransport(deviceId, sessionId, iceServers, onStatus, 1);
    }
    fail(reason); fire({ type: "control.close", deviceId, sessionId }); throw new Error(`WebRTC control unavailable: ${reason}`);
  }
  return {
    send(sequence, ciphertext) {
      if (channel.readyState !== "open" || channel.bufferedAmount > 1_048_576) return false;
      try { channel.send(JSON.stringify({ type: "control.frame", sessionId, sequence, ciphertext } satisfies Frame)); return true; }
      catch { fail("WebRTC send failed"); channel.close(); return false; }
    },
    onFrame(listener) { frameListeners.add(listener); return () => frameListeners.delete(listener); },
    close() { if (closing) return; closing = true; peer.close(); fire({ type: "control.close", deviceId, sessionId }); },
  };
}
