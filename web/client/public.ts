const toggle = document.querySelector<HTMLButtonElement>("[data-menu-toggle]");
const menu = document.querySelector<HTMLElement>("#mobile-menu");

function setOpen(open: boolean) {
  if (!toggle || !menu) return;
  menu.classList.toggle("open", open);
  toggle.setAttribute("aria-expanded", String(open));
  toggle.setAttribute("aria-label", open ? "Close menu" : "Open menu");
}

toggle?.addEventListener("click", () => setOpen(!menu?.classList.contains("open")));
menu?.querySelectorAll("a").forEach(link => link.addEventListener("click", () => setOpen(false)));
document.addEventListener("keydown", event => { if (event.key === "Escape") setOpen(false); });
