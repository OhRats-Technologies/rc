/** Native service coverage refuses to replace an existing user's RC task. */
export async function startWindowsNodeService(binary: string, state: string, components: string, kernel: string) {
  const existing = Bun.spawn(["schtasks.exe", "/Query", "/TN", "OhRats RC Node"], { stdout: "ignore", stderr: "ignore" });
  if (await existing.exited === 0) throw new Error("refusing to replace an existing RC scheduled task during E2E");
  async function command(action: string) {
    const child = Bun.spawn([binary, "service", action], {
      env: { ...Bun.env, RC_STATE_DIR: state, RC_KERNEL: kernel, RC_COMPONENT_DIR: components },
      stdout: "inherit", stderr: "inherit",
    });
    if (await child.exited !== 0) throw new Error(`RC service ${action} failed`);
  }
  try {
    await command("install");
    await command("status");
  } catch (error) {
    await command("uninstall").catch(() => {});
    throw error;
  }
  return {
    stop: () => command("stop"),
    start: () => command("start"),
    dispose: () => command("uninstall"),
  };
}
