const root = document.documentElement;
document.querySelector<HTMLButtonElement>("#sidebar-toggle")?.addEventListener("click", () => {
  const next = root.dataset.sidebar === "closed" ? "open" : "closed";
  root.dataset.sidebar = next;
  document.cookie = `relay_sidebar=${next}; Path=/; SameSite=Lax; Max-Age=31536000${location.protocol === "https:" ? "; Secure" : ""}`;
});

if (matchMedia("(max-width:760px)").matches && !document.cookie.includes("relay_sidebar=")) root.dataset.sidebar = "closed";
