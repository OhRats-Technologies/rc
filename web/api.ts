export class ApiError extends Error {
  constructor(message: string, readonly status: number) { super(message); }
}

export function qs<T extends Element>(selector: string, root: ParentNode = document) {
  const element = root.querySelector<T>(selector);
  if (!element) throw new Error(`Missing element: ${selector}`);
  return element;
}

export async function api<T>(path: string, options: RequestInit = {}): Promise<T> {
  const response = await fetch(path, {
    ...options,
    headers: { "content-type": "application/json", ...options.headers },
  });
  let data: { error?: string } & T;
  try { data = await response.json(); }
  catch { data = {} as typeof data; }
  if (!response.ok) throw new ApiError(data.error || response.statusText, response.status);
  return data;
}

export function escapeHTML(value: unknown = "") {
  return String(value).replace(/[&<>'"]/g, char => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", "'": "&#39;", '"': "&quot;",
  })[char] || char);
}

export function relative(timestamp: number | null | undefined) {
  if (!timestamp) return "NEVER";
  const seconds = Math.max(0, Math.round((Date.now() - timestamp) / 1000));
  if (seconds < 60) return `${seconds}S AGO`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}M AGO`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}H AGO`;
  return `${Math.floor(seconds / 86400)}D AGO`;
}

export async function copyText(value: string, button?: HTMLElement) {
  try {
    await navigator.clipboard.writeText(value);
  } catch {
    const textarea = document.createElement("textarea");
    textarea.value = value;
    textarea.style.position = "fixed";
    textarea.style.opacity = "0";
    document.body.append(textarea);
    textarea.select();
    document.execCommand("copy");
    textarea.remove();
  }
  if (!button) return;
  const original = button.textContent;
  button.textContent = "COPIED";
  setTimeout(() => { button.textContent = original; }, 1000);
}

export function formObject(form: HTMLFormElement) {
  return Object.fromEntries(new FormData(form).entries());
}
