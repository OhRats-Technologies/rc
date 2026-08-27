export function relative(timestamp: number | null | undefined) {
  if (!timestamp) return "NEVER";
  const seconds = Math.max(0, Math.round((Date.now() - timestamp) / 1000));
  if (seconds < 60) return `${seconds}S AGO`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}M AGO`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}H AGO`;
  return `${Math.floor(seconds / 86400)}D AGO`;
}

export function until(timestamp: number | null | undefined) {
  if (!timestamp) return "UNKNOWN";
  const seconds = Math.max(0, Math.round((timestamp - Date.now()) / 1000));
  if (seconds < 60) return `${seconds}S`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}M`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}H`;
  return `${Math.floor(seconds / 86400)}D`;
}

export function processState(process: { status: string; signal?: string | null; exit_code?: number | null }) {
  if (process.status === "starting") return "STARTING";
  if (process.status === "running") return "RUNNING";
  if (process.status === "lost") return "LOST";
  return process.signal || `EXIT ${process.exit_code ?? "?"}`;
}

export function processOriginLabel(origin: string) {
  switch (origin) {
    case "browser": return "BROWSER";
    case "cli": return "CLI";
    case "api": return "API";
    case "mcp": return "MCP";
    case "ssh": return "SSH";
    case "control": return "CONTROL";
    default: return "LEGACY";
  }
}

export function processLabel(process: { origin: string; terminal?: boolean }) {
  if (process.terminal) return "Terminal";
  switch (process.origin) {
    case "mcp": return "MCP process";
    case "ssh": return "SSH process";
    case "cli": return "CLI process";
    case "api": return "API process";
    case "browser": return "Browser process";
    default: return "Process";
  }
}
