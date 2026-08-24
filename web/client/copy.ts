import { copyText } from "./http";

document.querySelectorAll<HTMLElement>("[data-copy-value]").forEach(container => {
  container.querySelector<HTMLButtonElement>(".copy-value")?.addEventListener("click", event => {
    void copyText(container.dataset.copyValue || "", event.currentTarget as HTMLButtonElement);
  });
});
