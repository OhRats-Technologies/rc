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
