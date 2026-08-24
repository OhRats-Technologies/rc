export const $ = selector => document.querySelector(selector);

export async function api(path, options = {}) {
  const response = await fetch(path, {
    ...options,
    headers: { 'content-type': 'application/json', ...(options.headers || {}) },
  });
  let data = {};
  try { data = await response.json(); } catch {}
  if (!response.ok) throw Object.assign(new Error(data.error || response.statusText), { status: response.status });
  return data;
}

export function escapeHTML(value = '') {
  return String(value).replace(/[&<>'"]/g, char => ({ '&':'&amp;', '<':'&lt;', '>':'&gt;', "'":'&#39;', '"':'&quot;' }[char]));
}

export function relative(ts) {
  if (!ts) return 'NEVER';
  const seconds = Math.max(0, Math.round((Date.now() - ts) / 1000));
  if (seconds < 60) return `${seconds}S AGO`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}M AGO`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}H AGO`;
  return `${Math.floor(seconds / 86400)}D AGO`;
}

export async function copyText(value, button) {
  await navigator.clipboard.writeText(value);
  const original = button.textContent;
  button.textContent = 'COPIED';
  setTimeout(() => button.textContent = original, 1000);
}

export function formJSON(form) { return Object.fromEntries(new FormData(form).entries()); }
