import { copyText, qs } from "./http";
import { request } from "./socket";

const page = document.querySelector<HTMLElement>("[data-device-page]");
const deviceId = page?.dataset.devicePage || "";

async function start(command: string, cwd = "") {
  const result = await request<{ processId: string }>({ type: "process.start", deviceId, command, cwd, cols: 100, rows: 30 });
  location.href = `/devices/${deviceId}/processes/${result.processId}`;
}

document.querySelector<HTMLButtonElement>("#open-terminal")?.addEventListener("click", async () => {
  try { await start('exec "${SHELL:-sh}" -l'); }
  catch (error) { qs<HTMLElement>("#process-error").textContent = error instanceof Error ? error.message : String(error); }
});

document.querySelector<HTMLFormElement>("#process-launch")?.addEventListener("submit", async event => {
  event.preventDefault(); const form = event.currentTarget as HTMLFormElement;
  const command = qs<HTMLInputElement>("#process-command").value.trim(), cwd = qs<HTMLInputElement>("#process-cwd").value.trim();
  try { await start(command, cwd); }
  catch (error) { qs<HTMLElement>("#process-error").textContent = error instanceof Error ? error.message : String(error); }
});

document.querySelector<HTMLButtonElement>("#update-node")?.addEventListener("click", async event => {
  const button = event.currentTarget as HTMLButtonElement; button.disabled = true; qs<HTMLElement>("#update-state").textContent = "Starting update…";
  try { await request({ type: "node.update", deviceId }); qs<HTMLElement>("#update-state").textContent = "Updating and restarting…"; }
  catch (error) { qs<HTMLElement>("#update-state").textContent = error instanceof Error ? error.message : String(error); button.disabled = false; }
});

document.querySelector<HTMLButtonElement>("#copy-update")?.addEventListener("click", event => {
  void copyText(qs<HTMLElement>("#update-command").textContent || "", event.currentTarget as HTMLButtonElement);
});
