// Terminal rendering checks for the browser E2E run. xterm.js only lays out
// correctly when its bundled stylesheet is linked; without it the helper
// textarea covers the viewport and the screen has no size.

type Evaluate = <T>(expression: string) => Promise<T>;

type TerminalLayout = {
  hostH: number;
  screenW: number;
  screenH: number;
  helperW: number;
  helperH: number;
};

const layoutExpression = `(()=>{
  const box = element => { const rect = element?.getBoundingClientRect(); return { w: rect?.width ?? 0, h: rect?.height ?? 0 }; };
  const host = box(document.querySelector('.terminal-host')), screen = box(document.querySelector('.xterm-screen')), helper = box(document.querySelector('.xterm-helper-textarea'));
  return { hostH: host.h, screenW: screen.w, screenH: screen.h, helperW: helper.w, helperH: helper.h };
})()`;

export async function verifyTerminalRendering(evaluate: Evaluate) {
  const fonts = await evaluate<string[]>(
    `[getComputedStyle(document.querySelector('.terminal-host')).fontFamily,getComputedStyle(document.querySelector('.terminal-host .xterm')).fontFamily]`,
  );
  if (fonts.some(font => !font.includes("MesloLGS Nerd Font Mono"))) {
    throw new Error(`terminal font did not use Nerd Font: ${fonts.join(" / ")}`);
  }
  const layout = await evaluate<TerminalLayout>(layoutExpression);
  if (layout.helperW !== 0 || layout.helperH !== 0) {
    throw new Error(`terminal stylesheet missing; helper textarea is ${layout.helperW}x${layout.helperH}`);
  }
  if (layout.screenW < 200 || layout.screenH < 80) {
    throw new Error(`terminal did not lay out: ${JSON.stringify(layout)}`);
  }
}

type WaitFor = (expression: string, timeoutMs: number, label: string) => Promise<unknown>;

// The shell reports whether stdin is a terminal. A pipe-backed shell would
// report 0 and render no prompt, echo, or line editing.
export async function verifyTerminalTTY(evaluate: Evaluate, waitFor: WaitFor) {
  try {
    await waitFor(`String(window.__rcE2EOutput || '').includes('RC_BROWSER_E2E_TTY=')`, 20_000, "terminal TTY probe");
  } catch (failure) {
    const tail = await evaluate<string>(`String(window.__rcE2EOutput || "").slice(-500)`);
    throw new Error(`${String(failure)} output=${tail}`);
  }
  const tty = await evaluate<string>(`String(window.__rcE2EOutput || "").includes('RC_BROWSER_E2E_TTY=1') ? "1" : "0"`);
  if (tty !== "1") throw new Error("browser terminal is not attached to a PTY");
}
