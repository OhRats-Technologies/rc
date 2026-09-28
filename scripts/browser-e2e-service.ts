/** Native service coverage refuses to replace an existing user's RC task. */
export async function startWindowsNodeService(binary: string, state: string, components: string, kernel: string) {
  const existing = Bun.spawn(["schtasks.exe", "/Query", "/TN", "OhRats RC Node"], { stdout: "ignore", stderr: "ignore" });
  if (await existing.exited === 0) throw new Error("refusing to replace an existing RC scheduled task during E2E");
  // Prepare the same Wasm cache an installer prepares, outside the timed
  // service-start assertion. Also fail early if this fixture lacks rc logs.
  const prepare = Bun.spawn([kernel, "--component-dir", components, "commands"], {
    stdout: "pipe", stderr: "inherit",
  });
  const catalog = await new Response(prepare.stdout).text();
  if (await prepare.exited !== 0 || !/^  logs\s+.*\(ohrats:diagnostics-cli\)\r?$/m.test(catalog)) {
    throw new Error("Windows service fixture requires diagnostics-cli");
  }
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
    stop: async () => {
      await command("stop");
      const child = Bun.spawn([binary, "logs", "100"], {
        env: { ...Bun.env, RC_STATE_DIR: state, RC_KERNEL: kernel, RC_COMPONENT_DIR: components },
        stdout: "pipe", stderr: "pipe",
      });
      const output = await new Response(child.stdout).text();
      const error = await new Response(child.stderr).text();
      if (await child.exited !== 0 || !output.includes("service starting (windowless)")) {
        throw new Error(`Windows service logs unavailable: ${output} ${error}`);
      }
    },
    start: () => command("start"),
    dispose: async () => {
      await command("uninstall");
      const journal = Bun.file(`${state}/service.log`);
      if (await journal.exists()) console.log((await journal.text()).split("\n").slice(-80).join("\n"));
    },
  };
}
