export {};

const url = Bun.env.WEBVIEW_URL || "http://127.0.0.1:3000/";
const screenshot = Bun.env.WEBVIEW_SCREENSHOT || "";
const cookie = Bun.env.WEBVIEW_COOKIE || "";
const required = (Bun.env.WEBVIEW_REQUIRE || "").split(",").map(value => value.trim()).filter(Boolean);
const width = dimension("WEBVIEW_WIDTH", 1440);
const height = dimension("WEBVIEW_HEIGHT", 900);
const errors: string[] = [];

await using view = new Bun.WebView({
  width,
  height,
  console: (type, ...args) => {
    if (type === "error") errors.push(args.map(String).join(" "));
  },
});

view.onNavigationFailed = error => errors.push(`navigation: ${error.message}`);
await view.navigate(cookie ? new URL("/healthz", url).toString() : url);
if (cookie) {
  await view.evaluate(`document.cookie = ${JSON.stringify(cookie)}`);
  await view.navigate(url);
}
await Bun.sleep(300);

const state = await view.evaluate(`(async () => {
  const resources = [...performance.getEntriesByType("resource")]
    .map(entry => entry.name)
    .filter(value => value.startsWith(location.origin + "/assets/"));
  const declared = [...document.querySelectorAll('link[href^="/assets/"],script[src^="/assets/"]')]
    .map(element => element.href || element.src);
  const assets = [...new Set([...resources, ...declared])];
  const checks = await Promise.all(assets.map(async asset => {
    const response = await fetch(asset, { cache: "no-store" });
    return { asset, status: response.status };
  }));
  return {
    title: document.title,
    heading: document.querySelector("h1")?.textContent || "",
    overflowX: document.documentElement.scrollWidth > innerWidth,
    assets: checks,
    required: ${JSON.stringify(required)}.map(selector => ({ selector, present: Boolean(document.querySelector(selector)) })),
  };
})()`);

if (screenshot) await Bun.write(screenshot, await view.screenshot());

const result = state as {
  title: string;
  heading: string;
  overflowX: boolean;
  assets: Array<{ asset: string; status: number }>;
  required: Array<{ selector: string; present: boolean }>;
};
const failedAssets = result.assets.filter(asset => asset.status < 200 || asset.status >= 400);
const missing = result.required.filter(item => !item.present);

console.log(JSON.stringify({ url, width, height, ...result, errors, screenshot: screenshot || undefined }, null, 2));
if (!result.title || !result.heading || result.overflowX || failedAssets.length || missing.length || errors.length) process.exit(1);

function dimension(name: "WEBVIEW_WIDTH" | "WEBVIEW_HEIGHT", fallback: number): number {
  const parsed = Number.parseInt(Bun.env[name] || "", 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback;
}
