import { expect, test } from "bun:test";
import { waitForGathering } from "./control-webrtc";

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
