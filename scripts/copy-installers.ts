// Copy the canonical public installers into the web asset directory so the
// server can serve /install.sh and /install.ps1 from the same build artifact.
import { copyFile, mkdir } from "node:fs/promises";
import { join, resolve } from "node:path";

const root = resolve(import.meta.dir, "..");
await mkdir(join(root, "dist/assets"), { recursive: true });
for (const name of ["install.sh", "install.ps1"]) {
  await copyFile(join(root, "public", name), join(root, "dist/assets", name));
}
