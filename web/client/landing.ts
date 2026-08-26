const tabs = Array.from(document.querySelectorAll<HTMLButtonElement>("[data-resource-tab]"));
const panels = Array.from(document.querySelectorAll<HTMLElement>("[data-resource-panel]"));

function activate(name: string, updateHash = false) {
  const selected = tabs.some(tab => tab.dataset.resourceTab === name) ? name : "docs";
  for (const tab of tabs) tab.setAttribute("aria-selected", tab.dataset.resourceTab === selected ? "true" : "false");
  for (const panel of panels) panel.hidden = panel.dataset.resourcePanel !== selected;
  if (updateHash) history.replaceState(null, "", selected === "docs" ? "#docs" : `#${selected}`);
}

for (const tab of tabs) tab.addEventListener("click", () => activate(tab.dataset.resourceTab || "docs", true));

const initial = location.hash.slice(1);
if (["mcp", "api", "cli"].includes(initial)) activate(initial);
window.addEventListener("hashchange", () => activate(location.hash.slice(1)));

document.querySelector<HTMLFormElement>("[data-invite-start]")?.addEventListener("submit", event => {
  event.preventDefault();
  const form = event.currentTarget as HTMLFormElement;
  const raw = String(new FormData(form).get("invite") || "").trim();
  if (!raw) return;
  let token = raw;
  try { token = new URL(raw, location.origin).searchParams.get("invite") || raw; } catch {}
  location.assign(`/?invite=${encodeURIComponent(token)}`);
});
