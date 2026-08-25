import { api } from "./http";
import { passkeyAssertion } from "./webauthn";

export async function freshPasskey() {
  const start = await api<{ authorizationId: string; options: any }>("/api/v1/auth/step-up/options", { method: "POST", body: "{}" });
  const response = await passkeyAssertion(start.options);
  const result = await api<{ token: string }>("/api/v1/auth/step-up/verify", {
    method: "POST", body: JSON.stringify({ authorizationId: start.authorizationId, response }),
  });
  return result.token;
}

export function stepHeader(token: string) { return { "x-rc-step-up": token }; }
