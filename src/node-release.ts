const RELEASE_MANIFEST_URL = "https://github.com/OhRats-Technologies/rc/releases/latest/download/release.json";

let latestVersion = "";

export function currentNodeRelease() {
  return latestVersion;
}

export function parseNodeReleaseVersion(value: unknown) {
  if (!value || typeof value !== "object") return "";
  const version = String((value as { version?: unknown }).version || "");
  return /^\d+\.\d+\.\d+$/.test(version) ? version : "";
}

export async function refreshNodeRelease(fetcher: typeof fetch = fetch) {
  try {
    const response = await fetcher(RELEASE_MANIFEST_URL, { headers: { accept: "application/json" } });
    if (!response.ok) return latestVersion;
    const version = parseNodeReleaseVersion(await response.json());
    if (version) latestVersion = version;
  } catch {
    // Keep the last known release; update availability is advisory only.
  }
  return latestVersion;
}

export function startNodeReleaseRefresh() {
  void refreshNodeRelease();
  setInterval(() => void refreshNodeRelease(), 5 * 60_000).unref();
}
