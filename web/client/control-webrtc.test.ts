import { expect, test } from "bun:test";
import { directIceServers, hasTurnServer, peerConfiguration, waitForGathering } from "./control-webrtc";

class FakePeer extends EventTarget {
  iceGatheringState: RTCIceGatheringState = "gathering";
}

test("ICE gathering completion resolves immediately when the browser finishes", async () => {
  const peer = new FakePeer();
  const result = waitForGathering(peer as unknown as RTCPeerConnection, 100);
  peer.iceGatheringState = "complete";
  peer.dispatchEvent(new Event("icegatheringstatechange"));
  expect(await result).toBe("complete");
});

test("ICE gathering deadline keeps the partially gathered offer usable", async () => {
  const peer = new FakePeer();
  expect(await waitForGathering(peer as unknown as RTCPeerConnection, 1)).toBe("partial");
});

test("the first WebRTC attempt excludes TURN so a working direct route wins", () => {
  const servers: RTCIceServer[] = [{
    urls: ["stun:stun.cloudflare.com:3478", "turn:turn.cloudflare.com:3478?transport=udp"],
    username: "temporary-user",
    credential: "temporary-secret",
  }];
  const direct = [{
    urls: ["stun:stun.cloudflare.com:3478"],
    username: "temporary-user",
    credential: "temporary-secret",
  }];
  expect(directIceServers(servers)).toEqual(direct);
  expect(peerConfiguration(servers, 0)).toEqual({
    iceServers: direct,
    iceCandidatePoolSize: 1,
    iceTransportPolicy: "all",
  });
});

test("the retry becomes relay-only when TURN is available", () => {
  const servers: RTCIceServer[] = [{
    urls: ["stun:stun.cloudflare.com:3478", "turn:turn.cloudflare.com:3478?transport=udp"],
    username: "temporary-user",
    credential: "temporary-secret",
  }];
  expect(hasTurnServer(servers)).toBe(true);
  expect(peerConfiguration(servers, 1).iceTransportPolicy).toBe("relay");
});

test("the retry still permits host candidates when no TURN server exists", () => {
  const servers: RTCIceServer[] = [{ urls: ["stun:stun.cloudflare.com:3478"] }];
  expect(hasTurnServer(servers)).toBe(false);
  expect(peerConfiguration(servers, 1).iceTransportPolicy).toBe("all");
});
