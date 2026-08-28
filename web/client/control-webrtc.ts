import { closeControlSession, controlWebRTC } from "./control-api";
import type { ControlTransport } from "./control-transport";

type Frame = { type: "control.frame"; sessionId: string; sequence: number; ciphertext: string };
export type CandidateSummary = { host: number; srflx: number; relay: number; udp: number; tcp: number };
export type ControlTransportStatus = {
  transport: "webrtc"; phase?: "connecting" | "connected" | "failed"; reason?: string;
  iceState?: string; connectionState?: string; localCandidates?: CandidateSummary; remoteCandidates?: CandidateSummary;
  selected?: { localType?: string; remoteType?: string; protocol?: string };
};

type StatusReporter = (status: ControlTransportStatus) => void;

function serverUrls(server: RTCIceServer) {
  return typeof server.urls === "string" ? [server.urls] : server.urls;
}

export function hasTurnServer(iceServers: RTCIceServer[]) {
  return iceServers.some(server => serverUrls(server).some(url => /^turns?:/i.test(url)));
}

export function directIceServers(iceServers: RTCIceServer[]) {
  return iceServers.flatMap(server => {
    const urls = serverUrls(server).filter(url => !/^turns?:/i.test(url));
    if (!urls.length) return [];
    return [{ ...server, urls: typeof server.urls === "string" ? urls[0] : urls }];
  });
}

export function peerConfiguration(iceServers: RTCIceServer[], attempt = 0): RTCConfiguration {
  return {
    iceServers: attempt === 0 ? directIceServers(iceServers) : iceServers,
    iceCandidatePoolSize: 1,
    iceTransportPolicy: attempt > 0 && hasTurnServer(iceServers) ? "relay" : "all",
  };
}

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

type IceGatheringPeer = Pick<RTCPeerConnection, "iceGatheringState" | "addEventListener" | "removeEventListener">;

export function waitForGathering(peer: IceGatheringPeer, maxWaitMs = 15_000) {
  if (peer.iceGatheringState === "complete") return Promise.resolve("complete" as const);
  return new Promise<"complete" | "partial">(resolve => {
    const finish = (result: "complete" | "partial") => { cleanup(); resolve(result); };
    const timer = globalThis.setTimeout(() => finish("partial"), maxWaitMs);
    const change = () => { if (peer.iceGatheringState === "complete") finish("complete"); };
    const cleanup = () => { clearTimeout(timer); peer.removeEventListener("icegatheringstatechange", change); };
    peer.addEventListener("icegatheringstatechange", change);
  });
}

function candidateCount(value: CandidateSummary) {
  return value.host + value.srflx + value.relay;
}

function waitForOpen(peer: RTCPeerConnection, channel: RTCDataChannel, maxWaitMs = 15_000) {
  if (channel.readyState === "open") return Promise.resolve();
  return new Promise<void>((resolve, reject) => {
    const timer = window.setTimeout(() => {
      cleanup(); reject(new Error(`DataChannel timed out (ICE ${peer.iceConnectionState}, peer ${peer.connectionState})`));
    }, maxWaitMs);
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
      if (value.type === "transport" && value.selectedCandidatePairId) pair = stats.get(value.selectedCandidatePairId);
    });
    stats.forEach((value: any) => {
      if (!pair && value.type === "candidate-pair" && value.state === "succeeded" && (value.selected || value.nominated)) pair = value;
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
  onStatus: StatusReporter = () => {}, attempt = 0, fallbackReason?: string): Promise<ControlTransport> {
  if (typeof RTCPeerConnection === "undefined") throw new Error("WebRTC unavailable in this browser");
  const configuration = peerConfiguration(iceServers, attempt);
  const peer = new RTCPeerConnection(configuration), channel = peer.createDataChannel("rc-control", { ordered: true });
  const frameListeners = new Set<(sequence: number, ciphertext: string) => void>();
  let closing = false, established = false, localCandidates: CandidateSummary | undefined, remoteCandidates: CandidateSummary | undefined;

  const publish = (status: ControlTransportStatus) => onStatus(status);
  const fail = (reason: string) => publish({ transport: "webrtc", phase: "failed", reason: reason.slice(0, 200),
    iceState: peer.iceConnectionState, connectionState: peer.connectionState, localCandidates, remoteCandidates });
  publish({ transport: "webrtc", phase: "connecting" });

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
    if (!closing && established) { fail("WebRTC DataChannel closed"); closeControlSession(sessionId); }
  });
  peer.addEventListener("connectionstatechange", () => {
    if (!closing && established && peer.connectionState === "failed") {
      fail("WebRTC peer connection failed"); closing = true; closeControlSession(sessionId); channel.close(); peer.close();
    }
  });
  try {
    await peer.setLocalDescription(await peer.createOffer()); await waitForGathering(peer);
    const sdp = peer.localDescription?.sdp; if (!sdp) throw new Error("WebRTC offer unavailable");
    localCandidates = candidateSummary(sdp);
    if (candidateCount(localCandidates) === 0) throw new Error("browser ICE gathering produced no usable candidates");
    const answer = await controlWebRTC(sessionId, deviceId, sdp, attempt > 0);
    remoteCandidates = candidateSummary(answer.sdp);
    await peer.setRemoteDescription({ type: "answer", sdp: answer.sdp }); await waitForOpen(peer, channel);
    established = true;
    const selected = await selectedPair(peer);
    publish({ transport: "webrtc", phase: "connected", iceState: peer.iceConnectionState,
      connectionState: peer.connectionState, localCandidates, remoteCandidates, selected,
      reason: fallbackReason ? `Direct WebRTC failed before relay fallback: ${fallbackReason}` : undefined });
  } catch (error) {
    closing = true;
    const reason = error instanceof Error ? error.message : "WebRTC negotiation failed";
    peer.close();
    if (attempt === 0 && /webrtc|ice|datachannel|timed out|failed/i.test(reason)) {
      const mode = hasTurnServer(iceServers) ? " with TURN relay" : "";
      onStatus({ transport: "webrtc", phase: "connecting", reason: `Retrying${mode} after: ${reason}` });
      await new Promise(resolve => window.setTimeout(resolve, 1200));
      return openWebRTCControlTransport(deviceId, sessionId, iceServers, onStatus, 1, reason);
    }
    fail(reason); closeControlSession(sessionId); throw new Error(`WebRTC control unavailable: ${reason}`);
  }
  return {
    send(sequence, ciphertext) {
      if (channel.readyState !== "open" || channel.bufferedAmount > 1_048_576) return false;
      try { channel.send(JSON.stringify({ type: "control.frame", sessionId, sequence, ciphertext } satisfies Frame)); return true; }
      catch { fail("WebRTC send failed"); channel.close(); return false; }
    },
    onFrame(listener) { frameListeners.add(listener); return () => frameListeners.delete(listener); },
    close() { if (closing) return; closing = true; peer.close(); closeControlSession(sessionId); },
  };
}
