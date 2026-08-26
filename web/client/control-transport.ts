import { fire, onControlFrame } from "./socket";

export interface ControlTransport {
  send(sequence: number, ciphertext: string): boolean;
  onFrame(listener: (sequence: number, ciphertext: string) => void): () => void;
  onClose(listener: () => void): () => void;
  close(): void;
}

export function websocketControlTransport(deviceId: string, sessionId: string): ControlTransport {
  return {
    send: (sequence, ciphertext) => fire({ type: "control.frame", deviceId, sessionId, sequence, ciphertext }),
    onFrame(listener) {
      return onControlFrame(frame => {
        if (frame.sessionId === sessionId) listener(frame.sequence, frame.ciphertext);
      });
    },
    onClose: () => () => {},
    close: () => { fire({ type: "control.close", deviceId, sessionId }); },
  };
}
