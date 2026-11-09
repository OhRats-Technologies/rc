// Exercises the Remote MCP connection path in a real browser: dynamic client
// registration, PKCE authorization, passkey consent, RC Lock sync, a Node MCP
// execution, and grant revocation. It intentionally speaks raw JSON-RPC so the
// test covers the public protocol rather than an internal helper.

type Evaluate = <T>(expression: string) => Promise<T>;

export type MCPFlowOptions = {
  base: string;
  workspaceId: string;
  deviceId: string;
  evaluate: Evaluate;
  waitFor: (expression: string, timeoutMs: number, label: string) => Promise<unknown>;
  navigate: (url: string) => Promise<void>;
};

function base64url(value: ArrayBuffer | Uint8Array) {
  return Buffer.from(value instanceof Uint8Array ? value : new Uint8Array(value)).toString("base64url");
}

async function json<T>(url: string, options: RequestInit) {
  const response = await fetch(url, options);
  const text = await response.text();
  let body: any;
  try { body = JSON.parse(text); } catch { body = text; }
  if (!response.ok) throw new Error(`${url} failed (${response.status}): ${JSON.stringify(body)}`);
  return body as T;
}

export async function exerciseMCPConnection(options: MCPFlowOptions) {
  const { base, workspaceId, deviceId, evaluate, waitFor, navigate } = options;
  let callback: (params: URLSearchParams) => void = () => {};
  const received = new Promise<URLSearchParams>(resolve => { callback = resolve; });
  const listener = Bun.serve({
    hostname: "127.0.0.1",
    port: 0,
    fetch(request) {
      const url = new URL(request.url);
      callback(url.searchParams);
      return new Response("<!doctype html><title>RC MCP callback</title><p>Authorization received.</p>", {
        headers: { "content-type": "text/html; charset=utf-8", "cache-control": "no-store" },
      });
    },
  });
  const callbackURL = `http://127.0.0.1:${listener.port}/callback`;
  try {
    const registration = await json<{ client_id: string }>(`${base}/oauth/register`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({
        client_name: "RC Browser E2E MCP", application_type: "native",
        token_endpoint_auth_method: "none", redirect_uris: [callbackURL],
      }),
    });
    const verifier = base64url(crypto.getRandomValues(new Uint8Array(48)));
    const challenge = base64url(await crypto.subtle.digest("SHA-256", new TextEncoder().encode(verifier)));
    const state = `state-${crypto.randomUUID()}`;
    const authorize = new URL(`${base}/oauth/authorize`);
    authorize.search = new URLSearchParams({
      response_type: "code", client_id: registration.client_id, redirect_uri: callbackURL,
      state, code_challenge: challenge, code_challenge_method: "S256",
      resource: `${base}/mcp`, scope: "mcp:observe mcp:terminal",
    }).toString();

    await evaluate(`(()=>{location.href=${JSON.stringify(authorize.toString())};return true})()`);
    await waitFor(`document.querySelector('[data-mcp-form]') !== null`, 20_000, "MCP consent form");
    await evaluate(`(()=>{
      const terminal=document.querySelector('input[name="scope"][value="mcp:terminal"]');
      if(!terminal) throw new Error('terminal scope is missing');
      terminal.checked=true;
      const lifetime=document.querySelector('select[name="lifetime"]');
      if(lifetime) lifetime.value='1h';
      document.querySelector('[data-mcp-form]').requestSubmit();
      return true;
    })()`);

    const params = await Promise.race([
      received,
      new Promise<URLSearchParams>((_, reject) => setTimeout(() => reject(new Error("MCP OAuth callback timed out")), 45_000)),
    ]);
    if (params.get("error")) throw new Error(`MCP consent failed: ${params.get("error")} ${params.get("error_description") || ""}`);
    const code = params.get("code");
    if (!code || params.get("state") !== state) throw new Error(`MCP callback is invalid: ${params.toString()}`);

    const tokens = await json<{ access_token: string; refresh_token: string; scope: string }>(`${base}/oauth/token`, {
      method: "POST",
      headers: { "content-type": "application/x-www-form-urlencoded" },
      body: new URLSearchParams({
        grant_type: "authorization_code", code, redirect_uri: callbackURL,
        client_id: registration.client_id, code_verifier: verifier,
      }).toString(),
    });
    if (!tokens.access_token || !tokens.refresh_token || !tokens.scope.includes("mcp:terminal")) {
      throw new Error(`MCP token response is invalid: ${JSON.stringify(tokens)}`);
    }

    await navigate(`${base}/devices`);
    let sequence = 0;
    async function rpc(method: string, params: Record<string, unknown> = {}, version = "2025-06-18") {
      const response = await fetch(`${base}/mcp`, {
        method: "POST",
        headers: {
          Authorization: `Bearer ${tokens.access_token}`,
          "Content-Type": "application/json",
          Accept: "application/json, text/event-stream",
          "MCP-Protocol-Version": version,
        },
        body: JSON.stringify({ jsonrpc: "2.0", id: ++sequence, method, params }),
      });
      const text = await response.text();
      if (!response.ok) throw new Error(`MCP ${method} failed (${response.status}): ${text}`);
      const body = JSON.parse(text);
      if (body.error) throw new Error(`MCP ${method} error: ${JSON.stringify(body.error)}`);
      return body.result;
    }

    const initialized = await rpc("initialize", {
      protocolVersion: "2025-06-18", capabilities: {},
      clientInfo: { name: "RC Browser E2E MCP", version: "1.0.0" },
    });
    if (initialized.serverInfo?.name !== "RC") throw new Error(`MCP initialize = ${JSON.stringify(initialized)}`);
    const listed = await rpc("tools/list");
    const names = (listed.tools || []).map((tool: { name: string }) => tool.name);
    for (const required of ["machines_list", "process_run", "process_status", "process_input", "process_cancel", "image_view"]) {
      if (!names.includes(required)) throw new Error(`MCP tools/list is missing ${required}`);
    }
    const machines = await rpc("tools/call", { name: "machines_list", arguments: {} });
    const visible = machines.structuredContent?.machines || [];
    if (!visible.some((machine: { id: string }) => machine.id === deviceId)) {
      throw new Error(`MCP machines_list omitted ${deviceId}: ${JSON.stringify(machines)}`);
    }
    await waitFor(`(async()=>{const r=await fetch(${JSON.stringify(`/api/v1/workspaces/${workspaceId}/authority`)});if(!r.ok)return false;const j=await r.json();return j.synced===j.devices})()`, 30_000, "MCP grant RC Lock sync");

    let run = await rpc("tools/call", {
      name: "process_run",
      arguments: {
        deviceId, command: "echo RC_MCP_E2E_OK", shell: "system",
        waitSeconds: 20, maxRuntimeSeconds: 60,
      },
    });
    if (run.isError) throw new Error(`MCP process_run failed: ${JSON.stringify(run)}`);
    let result = run.structuredContent;
    for (let attempt = 0; result.status === "running" && attempt < 5; attempt++) {
      run = await rpc("tools/call", {
        name: "process_status",
        arguments: { deviceId, processId: result.processId, cursor: result.nextCursor, waitSeconds: 10 },
      });
      result = run.structuredContent;
    }
    if (result.status !== "exited" || !JSON.stringify(result.chunks || []).includes("RC_MCP_E2E_OK")) {
      throw new Error(`MCP process result is invalid: ${JSON.stringify(result)}`);
    }

    await navigate(`${base}/integrations/mcp`);
    await waitFor(`document.querySelector('[data-mcp-revoke]') !== null`, 15_000, "MCP grant page");
    await evaluate(`(()=>{document.querySelector('[data-mcp-revoke]').click();return true})()`);
    await waitFor(`document.querySelector('[data-mcp-revoke]') === null && document.body.innerText.includes('No MCP clients are connected')`, 20_000, "MCP grant revocation");
    const rejected = await fetch(`${base}/mcp`, {
      method: "POST",
      headers: { Authorization: `Bearer ${tokens.access_token}`, "Content-Type": "application/json" },
      body: JSON.stringify({ jsonrpc: "2.0", id: 99, method: "machines_list" }),
    });
    if (rejected.status !== 401) throw new Error(`revoked MCP token status = ${rejected.status}`);
  } finally {
    listener.stop(true);
  }
}
