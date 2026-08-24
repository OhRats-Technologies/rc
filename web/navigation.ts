let renderer: (() => Promise<void>) | null = null;

export function registerRenderer(value: () => Promise<void>) { renderer = value; }

export async function navigate(path: string, replace = false) {
  const target = new URL(path, location.href);
  if (target.origin !== location.origin) { location.href = target.href; return; }
  if (replace) history.replaceState(null, "", target.pathname + target.search + target.hash);
  else if (target.href !== location.href) history.pushState(null, "", target.pathname + target.search + target.hash);
  await renderer?.();
}
