import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import { mkdtempSync, readFileSync, writeFileSync } from 'node:fs';
import { createServer } from 'node:net';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';

// Visibility assertions need actual native hide/show/minimize callbacks. The
// ordinary background harness intentionally keeps hidden pages subscribed.
assert(process.argv.includes('--interactive'), 'Run with --interactive for native visibility checks; this briefly shows isolated mock windows.');
assert.equal(process.platform, 'darwin', 'This native visibility harness currently targets macOS.');
const root = join(import.meta.dirname, '..');
const productName = JSON.parse(readFileSync(join(root, 'package.json'), 'utf8')).productName;
const store = mkdtempSync(join(tmpdir(), 'daybee-window-resources-'));
const settings = JSON.parse(readFileSync(join(root, 'e2e/baseline/snapshot.json'), 'utf8')).settings;
Object.assign(settings.desktop, { launchAtLogin: false, voiceprintSetup: 'skipped' });
Object.assign(settings.recording, { outputDir: join(store, 'recordings'), disableAsr: false });
writeFileSync(join(store, 'desktop-settings.json'), JSON.stringify({ settings, nativeDevices: true }));
const port = await new Promise(resolve => {
  const server = createServer();
  server.listen(0, '127.0.0.1', () => { const p = server.address().port; server.close(() => resolve(p)); });
});
const token = randomBytes(32).toString('hex');
const target = `darwin-${process.arch === 'x64' ? 'amd64' : 'arm64'}`;
const child = spawn(join(root, 'dist', target, `${productName}.app`, 'Contents', 'MacOS', productName), [], {
  env: { ...process.env, DAYBEE_E2E_TEST: '1', DAYBEE_TEST_BACKGROUND: '0', DAYBEE_STORE_DIR: store, DAYBEE_TEST_PORT: String(port), DAYBEE_TEST_TOKEN: token },
  stdio: ['ignore', 'pipe', 'pipe'],
});
let logs = '';
child.stderr.on('data', b => { logs += b; });
const request = async (path, body, role = 'compact') => {
  const response = await fetch(`http://127.0.0.1:${port}${path}?window=${role}`, {
    method: 'POST', headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body), signal: AbortSignal.timeout(10000),
  });
  if (!response.ok) throw Error(`${path}: ${await response.text()}`);
  return response.json();
};
const evaluate = (code, role) => request('/eval', { code }, role);
const call = (method, args = [], role) => evaluate(`window.mygo.call(${JSON.stringify('DesktopService.' + method)}, ...${JSON.stringify(args)})`, role);
const until = async (fn, message) => {
  const deadline = Date.now() + 15000;
  while (Date.now() < deadline) {
    try { if (await fn()) return; } catch {}
    await delay(50);
  }
  throw Error(message + '\n' + logs);
};
const ready = role => until(() => evaluate(`Boolean(window.mygo && document.querySelector('[data-window=${role}]'))`, role), `window ${role} not ready`);
const listen = role => evaluate(`(() => {
  window.__resources = { full: 0, status: 0 };
  window.mygo.on('desktop:snapshot', snapshot => { window.__resources.full++; window.__latestResources = snapshot; });
  window.mygo.on('desktop:status', () => { window.__resources.status++; });
  return true;
})()`, role);
const counts = role => evaluate('window.__resources', role);
const lastText = role => evaluate('window.__latestResources?.status.asrPreview || ""', role);
const report = {};
try {
  await ready('compact');
  await listen('compact');
  await call('StartRecording');
  await delay(300);
  await request('/speech', { text: 'Visible compact window receives live speech.' });
  await until(async () => await lastText('compact') === 'Visible compact window receives live speech.', 'visible window did not receive speech');
  report.visibleCompact = true;
  await request('/hide', undefined, 'compact');
  await delay(300);
  const hiddenCompact = await counts('compact');
  await request('/speech', { text: 'Speech captured while the window is hidden.' });
  await delay(500);
  assert.deepEqual(await counts('compact'), hiddenCompact);
  assert.equal((await request('/snapshot')).status.recording, true);
  report.hiddenWindowEventDelta = { full: 0, status: 0 };
  await request('/show', undefined, 'compact');
  await until(async () => await lastText('compact') === 'Speech captured while the window is hidden.', 'reopened compact did not receive fresh state');
  report.reopenReceivedHiddenSpeech = true;

  await call('MinimizeWindow', ['compact'], 'compact');
  await delay(300);
  const minimized = await counts('compact');
  await request('/speech', { text: 'Speech captured while compact is minimized.' });
  await delay(500);
  assert.deepEqual(await counts('compact'), minimized);
  await request('/show', undefined, 'compact');
  await until(async () => await lastText('compact') === 'Speech captured while compact is minimized.', 'restored compact did not receive fresh state');
  report.minimizeAndRestore = true;

  await request('/hide-app', undefined, 'compact');
  await delay(300);
  const appHidden = await counts('compact');
  await request('/speech', { text: 'Speech captured while application is hidden.' });
  await delay(500);
  assert.deepEqual(await counts('compact'), appHidden);
  await request('/show', undefined, 'compact');
  await until(async () => await lastText('compact') === 'Speech captured while application is hidden.', 'unhidden app did not receive fresh state');
  report.applicationHideAndShow = true;
  await call('StopRecording', [], 'compact');
  writeFileSync(join(store, 'window-resources.json'), JSON.stringify(report, null, 2) + '\n');
  console.log(JSON.stringify(report));
} finally {
  child.kill('SIGTERM');
  await Promise.race([new Promise(resolve => child.once('exit', resolve)), delay(5000).then(() => child.kill('SIGKILL'))]);
}
