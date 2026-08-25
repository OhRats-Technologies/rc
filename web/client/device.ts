import { qs } from "./http";
import { request } from "./socket";

const page = document.querySelector<HTMLElement>("[data-device-page]");
const deviceId = page?.dataset.devicePage || "";

async function start(command: string, cwd = "") {
  const result = await request<{ processId: string }>({ type: "process.start", deviceId, command, cwd, cols: 80, rows: 24 });
  location.href = `/devices/${deviceId}/processes/${result.processId}`;
}

document.querySelector<HTMLButtonElement>("#open-terminal")?.addEventListener("click", async () => {
  try { await start('exec "${SHELL:-sh}" -l'); }
  catch (error) { qs<HTMLElement>("#process-error").textContent = error instanceof Error ? error.message : String(error); }
});
