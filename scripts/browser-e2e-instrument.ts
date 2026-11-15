// Injected before every page load to observe only the control data plane used
// by the browser E2E: decrypted process output and encrypted client frames.
export const instrumentSource = `    (() => {
      const originalEncrypt = SubtleCrypto.prototype.encrypt;
      const originalDecrypt = SubtleCrypto.prototype.decrypt;
      Object.defineProperty(window, "__rcE2EOutput", { value: "", writable: true, configurable: true });
      Object.defineProperty(window, "__rcE2ESent", { value: [], writable: true, configurable: true });
      Object.defineProperty(window, "__rcE2EKeyEvents", { value: 0, writable: true, configurable: true });
      document.addEventListener("keydown", () => { window.__rcE2EKeyEvents++; }, true);
      document.addEventListener("input", () => { window.__rcE2EKeyEvents++; }, true);
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
        const result = await originalDecrypt.apply(this, args);
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
