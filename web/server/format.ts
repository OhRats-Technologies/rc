export function relative(timestamp: number | null | undefined) {
  if (!timestamp) return "NEVER";
  const seconds = Math.max(0, Math.round((Date.now() - timestamp) / 1000));
  if (seconds < 60) return `${seconds}S AGO`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}M AGO`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}H AGO`;
  return `${Math.floor(seconds / 86400)}D AGO`;
}

export function processState(process: { status: string; signal?: string | null; exit_code?: number | null }) {
  if (process.status === "starting") return "STARTING";
  if (process.status === "running") return "RUNNING";
  if (process.status === "lost") return "LOST";
  return process.signal || `EXIT ${process.exit_code ?? "?"}`;
}
