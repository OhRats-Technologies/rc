import { STATIC_DIR } from "./config";
import { fail } from "./http-utils";

const downloadHashes = new Map<string, string>();

export async function frontendHTML(distDir: string) {
  const file = Bun.file(`${distDir}/web/index.html`);
  if (!await file.exists()) return fail("not found", 404);
  return new Response(file, {
    headers: {
      "content-type": "text/html; charset=utf-8",
      "cache-control": "public, max-age=0, must-revalidate",
    },
  });
}

export async function frontendAsset(distDir: string, name: string) {
  if (!/^index-[a-z0-9]+\.(js|css)$/.test(name)) return fail("not found", 404);
  const file = Bun.file(`${distDir}/${name}`);
  if (!await file.exists()) return fail("not found", 404);
  return new Response(file, {
    headers: {
      "content-type": name.endsWith(".css") ? "text/css; charset=utf-8" : "text/javascript; charset=utf-8",
      "cache-control": "public, max-age=31536000, immutable",
      "cloudflare-cdn-cache-control": "public, max-age=31536000, immutable",
    },
  });
}

async function fileHash(path: string) {
  const cached = downloadHashes.get(path);
  if (cached) return cached;
  const bytes = new Uint8Array(await Bun.file(path).arrayBuffer());
  const hash = new Bun.CryptoHasher("sha256").update(bytes).digest("hex").slice(0, 12);
  downloadHashes.set(path, hash);
  return hash;
}

export async function installScript() {
  const file = Bun.file(`${STATIC_DIR}/install.sh`);
  if (!await file.exists()) return fail("not found", 404);
  return new Response(file, {
    headers: {
      "content-type": "text/plain; charset=utf-8",
      "cache-control": "no-store",
      "cloudflare-cdn-cache-control": "no-store",
    },
  });
}

export async function download(name: string) {
  if (!/^ohrats-relay-(linux|darwin)-(amd64|arm64)(\.[0-9a-f]{12})?$/.test(name)) return fail("not found", 404);
  const match = name.match(/^(ohrats-relay-(?:linux|darwin)-(?:amd64|arm64))(?:\.([0-9a-f]{12}))?$/);
  if (!match) return fail("not found", 404);
  const [, logical, requestedHash] = match;
  const path = `${STATIC_DIR}/downloads/${logical}`;
  const file = Bun.file(path);
  if (!await file.exists()) return fail("not found", 404);
  const hash = await fileHash(path);
  if (!requestedHash) {
    return new Response(null, {
      status: 307,
      headers: {
        location: `/downloads/${logical}.${hash}`,
        "cache-control": "no-store",
        "cloudflare-cdn-cache-control": "no-store",
      },
    });
  }
  if (requestedHash !== hash) return fail("not found", 404);
  return new Response(file, {
    headers: {
      "content-type": "application/octet-stream",
      "cache-control": "public, max-age=31536000, immutable",
      "cloudflare-cdn-cache-control": "public, max-age=31536000, immutable",
    },
  });
}
