import { STATIC_DIR } from "./config";
import { fail } from "./http-utils";

const downloadHashes = new Map<string, string>();
const bundledAssetDir = `${import.meta.dir}/../assets`;
const sourceAssetDir = `${import.meta.dir}/../dist/assets`;
const assetDir = Bun.env.ASSET_DIR || (import.meta.dir.endsWith("/dist/server") ? bundledAssetDir : sourceAssetDir);
const assetCache = new Map<string, string>();

export function assetUrl(entry: string, extension = "js") {
  const key = `${entry}.${extension}`, cached = assetCache.get(key);
  if (cached) return cached;
  const match = Array.from(new Bun.Glob(`${entry}-*.${extension}`).scanSync({ cwd: assetDir }))[0];
  if (!match) return "";
  const url = `/assets/${match}`;
  assetCache.set(key, url);
  return url;
}

export async function frontendAsset(name: string) {
  if (!/^[a-zA-Z0-9._-]+\.(js|css|svg|png|webp)$/.test(name)) return fail("not found", 404);
  const file = Bun.file(`${assetDir}/${name}`);
  if (!await file.exists()) return fail("not found", 404);
  const types: Record<string, string> = {
    js: "text/javascript; charset=utf-8", css: "text/css; charset=utf-8", svg: "image/svg+xml",
    png: "image/png", webp: "image/webp",
  };
  const extension = name.split(".").pop() || "";
  return new Response(file, {
    headers: {
      "content-type": types[extension] || "application/octet-stream",
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
  if (name === "release.json" || name === "release.json.sig") {
    const file = Bun.file(`${STATIC_DIR}/downloads/${name}`);
    if (!await file.exists()) return fail("not found", 404);
    return new Response(file, {
      headers: {
        "content-type": name.endsWith(".json") ? "application/json; charset=utf-8" : "text/plain; charset=utf-8",
        "cache-control": "no-store",
        "cloudflare-cdn-cache-control": "no-store",
      },
    });
  }
  if (!/^ohrats-rc-(linux|darwin)-(amd64|arm64)(\.[0-9a-f]{12})?$/.test(name)) return fail("not found", 404);
  const match = name.match(/^(ohrats-rc-(?:linux|darwin)-(?:amd64|arm64))(?:\.([0-9a-f]{12}))?$/);
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
