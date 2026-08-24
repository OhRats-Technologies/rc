import { existsSync } from "node:fs";
import { extname, join } from "node:path";
import { fail } from "./http-utils";

function contentType(path: string) {
  return ({
    ".html": "text/html; charset=utf-8", ".js": "text/javascript; charset=utf-8",
    ".css": "text/css; charset=utf-8", ".svg": "image/svg+xml", ".png": "image/png",
    ".sh": "text/plain; charset=utf-8",
  } as Record<string, string>)[extname(path)] || "application/octet-stream";
}

export async function staticResponse(pathname: string) {
  let relative = pathname === "/" ? "index.html" : pathname.replace(/^\/+/, "");
  if (relative.includes("..")) return fail("not found", 404);
  let path = join(import.meta.dir, "..", "public", relative);
  if (!existsSync(path) && !extname(relative)) path = join(import.meta.dir, "..", "public", "index.html");
  if (!existsSync(path)) return fail("not found", 404);
  const immutable = pathname.startsWith("/downloads/") || pathname.startsWith("/icons/");
  const headers: Record<string, string> = {
    "content-type": contentType(path),
    "cache-control": immutable ? "public, max-age=86400" : "no-store, max-age=0",
  };
  if (!immutable) {
    headers["cdn-cache-control"] = "no-store";
    headers["cloudflare-cdn-cache-control"] = "no-store";
  }
  return new Response(Bun.file(path), { headers });
}
