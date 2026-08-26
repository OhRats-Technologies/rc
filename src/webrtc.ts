export type IceServer = { urls: string[]; username?: string; credential?: string };

export function controlIceServers(): IceServer[] {
  return [{ urls: ["stun:stun.cloudflare.com:3478"] }];
}
