const url = Bun.env.WEBVIEW_URL || "http://127.0.0.1:3000/";
const screenshot = Bun.env.WEBVIEW_SCREENSHOT || "";
const errors: string[] = [];

await using view = new Bun.WebView({
  width: 1440,
  height: 900,
  console: (type, ...args) => {
    if (type === "error") errors.push(args.map(String).join(" "));
  },
});

view.onNavigationFailed = error => errors.push(`navigation: ${error.message}`);
await view.navigate(url);
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
  };
})()`);

if (screenshot) await Bun.write(screenshot, await view.screenshot());

const result = state as {
  title: string;
  heading: string;
  overflowX: boolean;
  assets: Array<{ asset: string; status: number }>;
};
const failedAssets = result.assets.filter(asset => asset.status < 200 || asset.status >= 400);

console.log(JSON.stringify({ url, ...result, errors, screenshot: screenshot || undefined }, null, 2));
if (!result.title || !result.heading || result.overflowX || failedAssets.length || errors.length) process.exit(1);
