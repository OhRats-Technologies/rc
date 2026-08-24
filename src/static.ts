import { existsSync, statSync } from "node:fs";
import { extname, join } from "node:path";
import { fail } from "./http-utils";

const ROOT = join(import.meta.dir, "..");
const DIST = join(ROOT, "dist");
const PUBLIC = join(ROOT, "public");

function contentType(path: string) {
  return ({
    ".html": "text/html; charset=utf-8",
    ".js": "text/javascript; charset=utf-8",
    ".css": "text/css; charset=utf-8",
    ".svg": "image/svg+xml",
    ".png": "image/png",
    ".sh": "text/plain; charset=utf-8",
  } as Record<string, string>)[extname(path)] || "application/octet-stream";
}

function validators(path: string) {
  const stat = statSync(path);
  return `W/"${stat.size.toString(16)}-${Math.floor(stat.mtimeMs).toString(16)}"`;
}

function cacheHeaders(kind: "asset" | "html" | "download" | "other") {
  if (kind === "asset") return "public, max-age=31536000, immutable";
  if (kind === "download") return "public, max-age=3600, stale-while-revalidate=86400";
  if (kind === "html") return "public, max-age=0, must-revalidate";
  return "public, max-age=300, must-revalidate";
}

export async function staticResponse(req: Request, pathname: string) {
  if (pathname.includes("..")) return fail("not found", 404);

  let path: string;
  let kind: "asset" | "html" | "download" | "other";
  if (pathname.startsWith("/assets/")) {
    path = join(DIST, pathname.replace(/^\/+/, ""));
    kind = "asset";
  } else {
    const publicPath = join(PUBLIC, pathname.replace(/^\/+/, ""));
    if (pathname !== "/" && existsSync(publicPath)) {
      path = publicPath;
      kind = pathname.startsWith("/downloads/") ? "download" : "other";
    } else if (pathname === "/" || !extname(pathname)) {
      path = join(DIST, "index.html");
      kind = "html";
    } else {
      path = publicPath;
      kind = "other";
    }
  }
  if (!existsSync(path)) return fail("not found", 404);

  const headers: Record<string, string> = {
    "content-type": contentType(path),
    "cache-control": cacheHeaders(kind),
    "cloudflare-cdn-cache-control": cacheHeaders(kind),
  };
  if (kind !== "asset") {
    const etag = validators(path);
    headers.etag = etag;
    if (req.headers.get("if-none-match") === etag) return new Response(null, { status: 304, headers });
  }
  return new Response(Bun.file(path), { headers });
}
