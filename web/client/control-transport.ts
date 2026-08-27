export interface ControlTransport {
  send(sequence: number, ciphertext: string): boolean;
  onFrame(listener: (sequence: number, ciphertext: string) => void): () => void;
  close(): void;
}
