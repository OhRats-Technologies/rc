import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { $, api, escapeHTML, relative } from '../api.js';
import { onRelayEvent, relayRequest, relaySend } from '../events.js';

function terminalTheme() {
  const style = getComputedStyle(document.documentElement), value = name => style.getPropertyValue(name).trim();
  return { background: value('--or-surface'), foreground: value('--or-text'), cursor: value('--or-text'), selectionBackground: value('--or-surface-hover') };
}

function stateText(process) {
  if (process.status === 'starting') return 'STARTING';
  if (process.status === 'running') return 'RUNNING';
  if (process.status === 'lost') return 'LOST';
  return process.signal || `EXIT ${process.exit_code ?? '?'}`;
}

export async function renderProcess(deviceId, processId) {
  const [{ device }, { process }] = await Promise.all([api(`/api/v1/devices/${deviceId}`), api(`/api/v1/processes/${processId}`)]);
  if (process.device_id !== deviceId) throw new Error('Process not found');
  $('#page').innerHTML = `<section class="page process-page">
    <div class="page-heading">
      <div><p class="eyebrow"><a href="/devices/${deviceId}">${escapeHTML(device.name.toUpperCase())}</a> / PROCESS</p><h1 class="process-title mono">${escapeHTML(process.command)}</h1><div class="meta">${escapeHTML(process.cwd || '~')} · STARTED ${relative(process.created_at)}</div></div>
      <span id="process-state" class="status ${process.status === 'running' ? 'online' : ''}">${escapeHTML(stateText(process))}</span>
    </div>
    <section class="pty-panel">
      <div class="console-bar"><span>RELAY://NODE/${escapeHTML(device.name.toUpperCase())}/PTY/${process.id.slice(0, 8)}</span><div class="console-actions"><button class="text-button" data-signal="INT" type="button">CTRL-C</button><button class="text-button" data-signal="TERM" type="button">TERM</button><button class="text-button" data-signal="KILL" type="button">KILL</button></div></div>
      <div id="terminal-host" class="terminal-host dedicated-terminal"></div>
    </section>
    <p id="process-message" class="meta"></p>
  </section>`;

  const host = $('#terminal-host'), terminal = new Terminal({ cursorBlink: process.status === 'running', scrollback: 10000, theme: terminalTheme() });
  const fit = new FitAddon(); terminal.loadAddon(fit); terminal.open(host); terminal.write(process.output || ''); fit.fit();
  let current = process, lastRevision = Number(process.revision || 0);
  const resizeObserver = new ResizeObserver(() => fit.fit()); resizeObserver.observe(host);
  terminal.onData(data => { if (current.status === 'running') relaySend('process.input', { processId, data }); });
  terminal.onResize(size => { if (current.status === 'running') relaySend('process.resize', { processId, cols: size.cols, rows: size.rows }); });
  terminal.focus();

  async function resync() {
    const snapshot = (await api(`/api/v1/processes/${processId}`)).process;
    current = snapshot; lastRevision = Number(snapshot.revision || 0);
    terminal.reset(); terminal.write(snapshot.output || ''); terminal.options.cursorBlink = snapshot.status === 'running';
    const state = $('#process-state'); state.textContent = stateText(snapshot); state.classList.toggle('online', snapshot.status === 'running');
    if (snapshot.error) $('#process-message').textContent = snapshot.error;
  }

  document.querySelectorAll('[data-signal]').forEach(button => button.addEventListener('click', async () => {
    try { await relayRequest('process.signal', { processId, signal: button.dataset.signal }); }
    catch (error) { $('#process-message').textContent = error.message; }
  }));

  onRelayEvent(event => {
    if (event.kind === 'relay.connected') { resync().catch(() => {}); return; }
    if (event.processId !== processId) return;
    if (event.kind === 'process.output' && event.detail?.chunk) {
      const revision = Number(event.detail.revision || 0);
      if (!lastRevision || revision === lastRevision + 1) { terminal.write(event.detail.chunk); lastRevision = revision; }
      else resync().catch(() => {});
      return;
    }
    if (event.kind === 'process.started' || event.kind === 'process.exited' || event.kind === 'process.lost') resync().catch(() => {});
  });
}
