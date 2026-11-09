// Injected before every page load to observe only the control data plane used
// by the browser E2E: decrypted process output, encrypted client frames, ICE
// candidates, and the forced direct-route TURN failure.
export const instrumentSource = `    (() => {
      const original = SubtleCrypto.prototype.decrypt;
      const originalFetch = window.fetch.bind(window);
      Object.defineProperty(window, "__rcE2EOutput", { value: "", writable: true, configurable: true });
      Object.defineProperty(window, "__rcE2ESent", { value: [], writable: true, configurable: true });
      Object.defineProperty(window, "__rcE2EKeyEvents", { value: 0, writable: true, configurable: true });
      Object.defineProperty(window, "__rcE2ERtc", { value: { offers: [], answers: [] }, configurable: true });
      document.addEventListener("keydown", () => { window.__rcE2EKeyEvents++; }, true);
      document.addEventListener("input", () => { window.__rcE2EKeyEvents++; }, true);
      const summarizeCandidates = sdp => String(sdp || "").split(String.fromCharCode(10)).filter(line => line.startsWith("a=candidate:"))
        .map(line => { const fields = line.slice(12).trim().split(" ").filter(Boolean), typeAt = fields.indexOf("typ"), address = fields[4] || "";
          return { protocol: fields[2] || "", address: address.endsWith(".local") ? "mdns" : address.includes(":") ? "ipv6" : "ipv4", type: typeAt >= 0 ? fields[typeAt + 1] : "" }; });
      window.fetch = async (...args) => {
        const input = args[0], options = args[1] || {};
        const requestUrl = new URL(typeof input === "string" ? input : input.url, location.href);
        const signaling = requestUrl.pathname.startsWith("/api/v1/control/") && requestUrl.pathname.endsWith("/webrtc");
        if (signaling && typeof options.body === "string") {
          try { window.__rcE2ERtc.offers.push(summarizeCandidates(JSON.parse(options.body).sdp)); } catch {}
        }
        const response = await originalFetch(...args);
        if (signaling && response.ok) {
          try { window.__rcE2ERtc.answers.push(summarizeCandidates((await response.clone().json()).sdp)); } catch {}
        }
        if (requestUrl.pathname !== "/api/v1/control/open" || !response.ok) return response;
        const body = await response.clone().json();
        body.iceServers = [...(body.iceServers || []), {
          urls: "turn:127.0.0.1:9?transport=udp", username: "e2e", credential: "unreachable",
        }];
        const headers = new Headers(response.headers); headers.delete("content-length");
        return new Response(JSON.stringify(body), { status: response.status, statusText: response.statusText, headers });
      };
      const originalEncrypt = SubtleCrypto.prototype.encrypt;
      SubtleCrypto.prototype.encrypt = async function(...args) {
        try {
          const message = JSON.parse(new TextDecoder().decode(args[2]));
          let data = "";
          if (message.data) {
            try { data = new TextDecoder().decode(Uint8Array.from(atob(String(message.data).replace(/-/g, "+").replace(/_/g, "/")), value => value.charCodeAt(0))); } catch {}
          }
          window.__rcE2ESent.push({ type: message.type, id: message.id, data });
        } catch {}
        return await originalEncrypt.apply(this, args);
      };
      SubtleCrypto.prototype.decrypt = async function(...args) {
        const result = await original.apply(this, args);
        try {
          const message = JSON.parse(new TextDecoder().decode(result));
          if ((message.type === "process.stdout" || message.type === "process.stderr") && message.data) {
            const base64 = String(message.data).replace(/-/g, "+").replace(/_/g, "/");
            const padded = base64 + "=".repeat((4 - base64.length % 4) % 4);
            const bytes = Uint8Array.from(atob(padded), value => value.charCodeAt(0));
            window.__rcE2EOutput += new TextDecoder().decode(bytes);
          }
        } catch {}
        return result;
      };
    })();`;
