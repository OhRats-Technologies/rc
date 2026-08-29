const root = document.documentElement;
const toggle = document.querySelector("[data-sidebar-toggle]");

toggle?.addEventListener("click", () => {
  const state = root.dataset.sidebar === "closed" ? "open" : "closed";
  root.dataset.sidebar = state;
  document.cookie = `rc_sidebar=${state}; Path=/; SameSite=Lax`;
});

for (const button of document.querySelectorAll("[data-workspace-toggle]")) {
  button.addEventListener("click", () => {
    const folder = button.closest("[data-workspace-folder]");
    const children = folder?.querySelector("[data-workspace-children]");
    const open = button.getAttribute("aria-expanded") !== "true";
    button.setAttribute("aria-expanded", String(open));
    if (children instanceof HTMLElement) {
      children.hidden = !open;
      children.dataset.open = String(open);
    }
  });
}

const createTrigger = document.querySelector("[data-workspace-create-trigger]");
const createForm = document.querySelector("[data-workspace-create-form]");
const createInput = createForm?.querySelector("input[name=name]");

const cancelCreate = () => {
  if (!(createForm instanceof HTMLFormElement)) return;
  createForm.hidden = true;
  createForm.reset();
};

createTrigger?.addEventListener("click", () => {
  if (!(createForm instanceof HTMLFormElement)) return;
  createForm.hidden = false;
  if (createInput instanceof HTMLInputElement) createInput.focus();
});
createInput?.addEventListener("keydown", (event) => {
  if (event.key === "Escape") cancelCreate();
});
createInput?.addEventListener("blur", () => {
  if (createInput instanceof HTMLInputElement && createInput.value.trim() === "") {
    cancelCreate();
  }
});
