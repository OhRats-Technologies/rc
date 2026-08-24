export class ApiError extends Error {
  constructor(message: string, readonly status: number) { super(message); }
}

export async function api<T>(path: string, options: RequestInit = {}): Promise<T> {
  const response = await fetch(path, { ...options, headers: { "content-type": "application/json", ...options.headers } });
  let data: { error?: string } & T;
  try { data = await response.json(); } catch { data = {} as typeof data; }
  if (!response.ok) throw new ApiError(data.error || response.statusText, response.status);
  return data;
}

export function qs<T extends Element>(selector: string, root: ParentNode = document) {
  const element = root.querySelector<T>(selector);
  if (!element) throw new Error(`Missing element: ${selector}`);
  return element;
}

export async function copyText(value: string, button?: HTMLElement) {
  await navigator.clipboard.writeText(value);
  if (!button) return;
  const original = button.textContent;
  button.textContent = "COPIED";
  setTimeout(() => { button.textContent = original; }, 1000);
}
