import { createHash } from "node:crypto";
import { mkdirSync, readdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { basename, extname, join } from "node:path";

const ROOT = import.meta.dir;
const PUBLIC = join(ROOT, "public");
const DIST = join(ROOT, "dist");
const ASSETS = join(DIST, "assets");

function digest(value: string | Uint8Array) {
  return createHash("sha256").update(value).digest("hex").slice(0, 12);
}

function hashedName(name: string, value: string | Uint8Array) {
  const extension = extname(name);
  const stem = basename(name, extension);
  return `${stem}.${digest(value)}${extension}`;
}

rmSync(DIST, { recursive: true, force: true });
mkdirSync(ASSETS, { recursive: true });

const iconPaths = new Map<string, string>();
for (const name of readdirSync(join(PUBLIC, "icons")).filter((name) => name.endsWith(".svg"))) {
  const source = readFileSync(join(PUBLIC, "icons", name));
  const output = hashedName(name, source);
  writeFileSync(join(ASSETS, output), source);
  iconPaths.set(`/icons/${name}`, `/assets/${output}`);
}

let css = readFileSync(join(PUBLIC, "relay.css"), "utf8");
for (const [from, to] of iconPaths) css = css.split(from).join(to);
const cssName = hashedName("relay.css", css);
writeFileSync(join(ASSETS, cssName), css);

const build = await Bun.build({
  entrypoints: [join(PUBLIC, "app.js")],
  target: "browser",
  minify: true,
  splitting: false,
  write: false,
});
if (!build.success) throw new Error("frontend build failed");
const script = new Uint8Array(await build.outputs[0].arrayBuffer());
const scriptName = hashedName("app.js", script);
writeFileSync(join(ASSETS, scriptName), script);

const html = readFileSync(join(PUBLIC, "index.html"), "utf8")
  .replace(/\/relay\.css(?:\?v=\d+)?/, `/assets/${cssName}`)
  .replace(/\/app\.js(?:\?v=\d+)?/, `/assets/${scriptName}`);
writeFileSync(join(DIST, "index.html"), html);

console.log(`Relay assets: ${scriptName}, ${cssName}`);
