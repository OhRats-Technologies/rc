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

export const LOGIN_SHELL_COMMAND = 'exec "${SHELL:-sh}" -l';
export function processLabel(command: string) {
  if (command === "[encrypted]") return "Encrypted terminal";
  return command === LOGIN_SHELL_COMMAND ? "Terminal" : command;
}

export function terminalFallback(value: string) {
  return String(value || "")
    .replace(/\x1B\][\s\S]*?(?:\x07|\x1B\\)/g, "")
    .replace(/\x1B\[[0-?]*[ -/]*[@-~]/g, "")
    .replace(/\x1B[()][0-2A-Z0-9]/g, "")
    .replace(/\x1B[=>]/g, "")
    .replace(/\r\n/g, "\n")
    .replace(/\r/g, "\n")
    .replace(/[\x00-\x08\x0B\x0C\x0E-\x1F\x7F]/g, "");
}
