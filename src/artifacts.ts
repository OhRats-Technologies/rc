import { STATIC_DIR } from "./config";
import { fail } from "./http-utils";

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
