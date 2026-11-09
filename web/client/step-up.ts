import { api } from "./http";
import { passkeyAssertion } from "./webauthn";

export async function freshPasskey() {
  const start = await api<{ ceremonyId: string; options: any }>("/api/v1/auth/step-up/options", { method: "POST", body: "{}" });
  const response = await passkeyAssertion(start.options);
  await api<{ ok: boolean }>("/api/v1/auth/step-up/verify", {
    method: "POST", body: JSON.stringify({ ceremonyId: start.ceremonyId, response }),
  });
  return "";
}

export function stepHeader(token: string): Record<string, string> {
  return token ? { "x-rc-step-up": token } : {};
}
