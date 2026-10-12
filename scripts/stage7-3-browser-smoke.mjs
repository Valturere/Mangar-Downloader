// Installed Chromium + genuine toolbar popup + native helper + production Wails.
// Run via stage7-3-smoke.ps1 to restore the temporary HKCU host registration.
import { chromium } from '../frontend/node_modules/playwright/index.mjs';
import { createServer } from 'node:http';
import { spawn, spawnSync } from 'node:child_process';
import { randomUUID, createHash } from 'node:crypto';
import * as fs from 'node:fs';
import path from 'node:path';
import assert from 'node:assert/strict';
const channel = process.argv[2] || 'chrome';
const localOnly = process.argv.includes('--local-only');
assert(['chrome', 'msedge'].includes(channel));
const root = path.resolve(import.meta.dirname, '..');
const binary =
  process.env.MANGAR_BROWSER_SMOKE_BIN || path.join(root, 'build/bin/stage7-3');
const runRoot = path.join(
  root,
  '.local/stage7-3',
  channel + '-' + randomUUID(),
);
const profile = path.join(runRoot, 'profile'),
  output = path.join(runRoot, 'output');
const appRoot = path.join(profile, 'YTDLPGUIMangar');
fs.mkdirSync(output, { recursive: true });
const source = path.join(process.env.LOCALAPPDATA, 'YTDLPGUIMangar/tools');
const exes = {};
function fingerprint() {
  const result = {};
  for (const component of ['yt-dlp', 'ffmpeg', 'deno']) {
    const current = path.join(source, component, 'current.json');
    const m = JSON.parse(fs.readFileSync(current));
    result[component] = createHash('sha256')
      .update(fs.readFileSync(current))
      .digest('hex');
    for (const name of Object.keys(m.file_sha256))
      result[component + '/' + m.identity + '/' + name] = createHash('sha256')
        .update(
          fs.readFileSync(
            path.join(source, component, 'versions', m.identity, name),
          ),
        )
        .digest('hex');
  }
  return JSON.stringify(result);
}
const before = fingerprint();
for (const component of ['yt-dlp', 'ffmpeg', 'deno']) {
  const current = path.join(source, component, 'current.json');
  const m = JSON.parse(fs.readFileSync(current));
  const dest = path.join(appRoot, 'tools', component);
  fs.mkdirSync(dest, { recursive: true });
  fs.copyFileSync(current, path.join(dest, 'current.json'));
  fs.cpSync(
    path.join(source, component, 'versions', m.identity),
    path.join(dest, 'versions', m.identity),
    { recursive: true },
  );
  for (const [name, rel] of Object.entries(m.executables))
    exes[name] = path.join(dest, 'versions', m.identity, rel);
}
const delay = (ms) => new Promise((r) => setTimeout(r, ms));
async function wait(fn, label, timeout = 30000) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    const r = await fn();
    if (r) return r;
    await delay(150);
  }
  throw Error(label);
}
function run(exe, args) {
  const r = spawnSync(exe, args, {
    windowsHide: true,
    encoding: 'utf8',
    timeout: 30000,
  });
  assert.equal(r.status, 0, 'media fixture/probe failed');
  return r.stdout;
}
const fixture = path.join(runRoot, 'fixture.mp4');
run(exes['ffmpeg.exe'], [
  '-hide_banner',
  '-nostdin',
  '-y',
  '-f',
  'lavfi',
  '-i',
  'testsrc2=size=160x90:rate=12',
  '-f',
  'lavfi',
  '-i',
  'sine=frequency=440:sample_rate=44100',
  '-t',
  '3',
  '-c:v',
  'libx264',
  '-c:a',
  'aac',
  fixture,
]);
const server = createServer((req, res) => {
  if (req.url.startsWith('/fixture.mp4')) {
    res.setHeader('Content-Type', 'video/mp4');
    fs.createReadStream(fixture).pipe(res);
  } else {
    res.setHeader('Content-Type', 'text/html; charset=utf-8');
    res.end(
      req.url.startsWith('/unsupported')
        ? '<!doctype html><title>Нет видео</title><p>Обычная страница</p>'
        : '<!doctype html><title>Engine title Mangar</title><video controls src="/fixture.mp4"></video>',
    );
  }
});
await new Promise((r) => server.listen(0, '127.0.0.1', r));
const origin = `http://127.0.0.1:${server.address().port}`;
const env = { ...process.env, LOCALAPPDATA: profile };
function native(action = 'Snapshot', url = '') {
  const r = spawnSync(
    'pwsh',
    [
      '-NoProfile',
      '-File',
      path.join(root, 'scripts/stage7-2-native.ps1'),
      '-Profile',
      profile,
      '-Action',
      action,
      '-Directory',
      output,
      '-Executable',
      path.join(binary, 'YTDLP-GUI-Mangar.exe'),
      '-URL',
      url,
    ],
    { windowsHide: true, encoding: 'utf8', timeout: 30000 },
  );
  if (r.status !== 0) throw Error('native ' + action + ' failed');
  return JSON.parse(r.stdout.trim());
}
function logs() {
  const dir = path.join(appRoot, 'logs/jobs');
  return fs.existsSync(dir)
    ? fs
        .readdirSync(dir)
        .filter((n) => n.endsWith('.log'))
        .map((n) => ({
          id: n,
          text: fs.readFileSync(path.join(dir, n), 'utf8'),
        }))
    : [];
}
function files() {
  return fs.readdirSync(output).filter((n) => n.endsWith('.mp4'));
}
async function outcome(oldLogs, oldFiles, label, timeout = 60000) {
  let timedOut = false;
  const entry = await wait(
    () =>
      logs().find(
        (e) =>
          !oldLogs.includes(e.id) &&
          /"phase":"(?:finish|completed)"/.test(e.text),
      ),
    label + ' finish',
    timeout,
  ).catch(async () => {
    timedOut = true;
    native('Cancel');
    native('Hide');
    return await wait(
      () =>
        logs().find(
          (e) => !oldLogs.includes(e.id) && /"phase":"finish"/.test(e.text),
        ),
      'cancel cleanup',
    );
  });
  const file = files().find((n) => !oldFiles.includes(n));
  const metadataLines = entry.text
    .split('\n')
    .filter(Boolean)
    .map((line) => JSON.parse(line))
    .filter(
      (record) =>
        record.stream === 'stdout' && record.detail.startsWith('MANGAR_META:'),
    );
  let metadataTitle;
  try {
    metadataTitle = JSON.parse(
      metadataLines.at(-1).detail.slice('MANGAR_META:'.length),
    ).title;
  } catch {}
  const result = {
    completed: !!file,
    timedOut,
    metadata: metadataLines.length > 0,
    unsupported: /Unsupported URL/.test(entry.text),
    auth: /Sign in|cookies|HTTP Error 403/i.test(entry.text),
    title: metadataTitle,
  };
  if (file) {
    const p = JSON.parse(
      run(exes['ffprobe.exe'], [
        '-v',
        'error',
        '-show_format',
        '-show_streams',
        '-of',
        'json',
        path.join(output, file),
      ]),
    );
    assert(p.format.format_name.includes('mp4'));
    assert(
      p.streams.some((s) => s.codec_type === 'video') &&
        p.streams.some((s) => s.codec_type === 'audio'),
    );
    result.streams = p.streams.map((s) => ({
      type: s.codec_type,
      codec: s.codec_name,
    }));
    result.duration = p.format.duration;
    result.metadataFilename =
      file.startsWith('Me at the zoo') ||
      file.startsWith('Engine title Mangar');
  }
  console.log(label + ': ' + JSON.stringify(result));
  return result;
}
let context, cdp, id, popup, appPID;
const results = {};
async function toolbar(page) {
  if (popup) await popup.close();
  await page.bringToFront();
  const { targetInfos } = await cdp.send('Target.getTargets', {
    filter: [{ type: 'tab', exclude: false }, { exclude: true }],
  });
  const tab = targetInfos.find((t) => t.url === page.url());
  assert(tab, 'active tab target');
  await cdp.send('Extensions.triggerAction', { id, targetId: tab.targetId });
  const target = await wait(async () => {
    const { targetInfos } = await cdp.send('Target.getTargets', {
      filter: [{ exclude: false }],
    });
    return targetInfos.find((t) => t.url.endsWith('/popup.html'));
  }, 'popup absent');
  const { sessionId } = await cdp.send('Target.attachToTarget', {
    targetId: target.targetId,
    flatten: false,
  });
  let sequence = 0;
  const command = (method, params) =>
    new Promise((resolve, reject) => {
      const messageId = ++sequence;
      const timer = setTimeout(() => {
        cdp.off('Target.receivedMessageFromTarget', receive);
        reject(Error('popup timeout'));
      }, 10000);
      function receive(e) {
        if (e.sessionId !== sessionId) return;
        const m = JSON.parse(e.message);
        if (m.id !== messageId) return;
        clearTimeout(timer);
        cdp.off('Target.receivedMessageFromTarget', receive);
        if (m.error || m.result.exceptionDetails)
          reject(Error('popup evaluation failed'));
        else resolve(m.result);
      }
      cdp.on('Target.receivedMessageFromTarget', receive);
      cdp
        .send('Target.sendMessageToTarget', {
          sessionId,
          message: JSON.stringify({
            id: messageId,
            method,
            params,
          }),
        })
        .catch(reject);
    });
  const evaluate = async (expression) =>
    (
      await command('Runtime.evaluate', {
        expression,
        returnByValue: true,
        awaitPromise: true,
      })
    ).result.value;
  popup = {
    evaluate,
    close: () =>
      cdp
        .send('Target.closeTarget', { targetId: target.targetId })
        .catch(() => {}),
  };
  await wait(
    () => evaluate('!document.querySelector("#send-page").disabled'),
    'page action absent',
  );
  const text = await evaluate('document.body.innerText');
  assert(!text.includes('Видео не обнаружено'));
  assert(!text.includes('QUERY_SECRET'));
  const screenshot = await command('Page.captureScreenshot', { format: 'png' });
  fs.writeFileSync(
    path.join(runRoot, `popup-${Object.keys(results).length}.png`),
    Buffer.from(screenshot.data, 'base64'),
  );
  await evaluate(
    'document.querySelector("#send-page").click();document.querySelector("#send-page").click();true',
  );
  await wait(
    () =>
      evaluate(
        'document.querySelector("#status").textContent==="Передано в Mangar"',
      ),
    'page handoff',
    20000,
  );
}
async function sendURL(url, label) {
  const visibleBefore = native().visible;
  const page = await context.newPage();
  await page.goto(url, { waitUntil: 'domcontentloaded', timeout: 30000 });
  const oldLogs = logs().map((e) => e.id),
    oldFiles = files();
  await toolbar(page);
  assert.equal(
    native().visible,
    visibleBefore,
    'page handoff changed existing window visibility',
  );
  const r = await outcome(oldLogs, oldFiles, label);
  await popup.close();
  popup = null;
  await page.close();
  return r;
}
try {
  context = await chromium.launchPersistentContext(
    path.join(
      profile,
      channel === 'chrome'
        ? 'Google/Chrome/User Data'
        : 'Microsoft/Edge/User Data',
    ),
    {
      channel,
      headless: false,
      ignoreDefaultArgs: ['--disable-extensions'],
      env,
      args: ['--enable-unsafe-extension-debugging'],
    },
  );
  cdp = await context.browser().newBrowserCDPSession();
  ({ id } = await cdp.send('Extensions.loadUnpacked', {
    path: path.join(root, 'browser-extension'),
  }));
  // Cold app: pending page URL through genuine toolbar popup, then native picker.
  const page = await context.newPage();
  await page.goto(origin + '/watch?token=QUERY_SECRET');
  await toolbar(page);
  const initial = await wait(() => {
    try {
      const s = native();
      return s.visible &&
        s.names.includes(
          'Ссылка получена из браузера. Выберите папку для загрузки.',
        )
        ? s
        : false;
    } catch {
      return false;
    }
  }, 'pending folder prompt');
  assert.equal(logs().length, 0);
  appPID = initial.pid;
  native('Folder');
  results.generic = await outcome([], [], 'generic controlled');
  assert(results.generic.completed && results.generic.metadataFilename);
  assert.equal(logs().length, 1, 'double click created two jobs');
  await popup.close();
  popup = null;
  native('Hide');
  await wait(() => !native().visible, 'tray hide');
  if (localOnly)
    results.genericTray = await sendURL(
      origin + '/watch?case=tray',
      'generic tray',
    );
  results.youtube = localOnly
    ? { skipped: 'targeted local-only lifecycle rerun' }
    : await sendURL('https://www.youtube.com/watch?v=jNQXAC9IVRw', 'YouTube');
  assert.equal(native().visible, false, 'tray page handoff revealed window');
  assert.equal(native().pid, initial.pid);
  const second = spawn(path.join(binary, 'YTDLP-GUI-Mangar.exe'), [], {
    env,
    windowsHide: true,
    stdio: 'ignore',
  });
  await wait(() => second.exitCode !== null, 'second launch exit');
  assert.equal(second.exitCode, 0);
  assert.equal(native().pid, initial.pid);
  assert.equal(native().windowCount, 1);
  // Ordinary manual paste: same source/engine result as extension, genuine UIA.
  const oldLogs = logs().map((e) => e.id),
    oldFiles = files();
  native(
    'Paste',
    localOnly
      ? origin + '/watch'
      : 'https://www.youtube.com/watch?v=jNQXAC9IVRw',
  );
  results.manual = await outcome(
    oldLogs,
    oldFiles,
    localOnly ? 'manual generic' : 'manual YouTube',
  );
  if (localOnly) assert.equal(results.manual.completed, true);
  else assert.equal(results.manual.completed, results.youtube.completed);
  results.unsupported = await sendURL(origin + '/unsupported', 'unsupported');
  assert.equal(results.unsupported.completed, false);
  // Shorts route uses the same extractor; a real Shorts URL can be supplied.
  if (!localOnly && process.env.MANGAR_SHORTS_PAGE)
    results.shorts = await sendURL(process.env.MANGAR_SHORTS_PAGE, 'Shorts');
  else results.shorts = { skipped: 'real Shorts URL not supplied' };
  if (!localOnly && process.env.MANGAR_GENERIC_PAGE)
    results.publicGeneric = await sendURL(
      process.env.MANGAR_GENERIC_PAGE,
      'public generic',
    );
  const temp = path.join(appRoot, 'temp');
  assert(
    !fs.existsSync(temp) || fs.readdirSync(temp).length === 0,
    'temp leaked',
  );
  assert(
    logs().every((e) => !e.text.includes('QUERY_SECRET')),
    'query log leaked',
  );
  results.native = {
    closed: true,
    pendingFolder: true,
    tray: true,
    secondInstance: true,
    manual: true,
    doubleClick: true,
  };
  results.browser = context.browser().version();
  fs.writeFileSync(
    path.join(runRoot, 'result.json'),
    JSON.stringify(results, null, 2),
  );
  console.log('artifacts: ' + runRoot);
} finally {
  let exitError, exitState;
  try {
    exitState = native('Exit');
  } catch (error) {
    exitError = error;
  }
  if (context) await context.close();
  await new Promise((r) => server.close(r));
  assert.equal(fingerprint(), before, 'Production tools changed');
  // No children may survive a successful native Exit.
  const r = spawnSync(
    'pwsh',
    [
      '-NoProfile',
      '-Command',
      `@(Get-CimInstance Win32_Process | Where-Object { $_.ExecutablePath -and $_.ExecutablePath.StartsWith('${appRoot.replaceAll("'", "''")}',[StringComparison]::OrdinalIgnoreCase) }).Count`,
    ],
    { windowsHide: true, encoding: 'utf8' },
  );
  assert.equal(r.stdout.trim(), '0', 'isolated tool orphan');
  if (appPID) {
    const appCheck = spawnSync(
      'pwsh',
      [
        '-NoProfile',
        '-Command',
        `@(Get-Process -Id ${appPID} -ErrorAction SilentlyContinue).Count`,
      ],
      { windowsHide: true, encoding: 'utf8' },
    );
    assert.equal(appCheck.stdout.trim(), '0', 'isolated App orphan');
    results.exit = exitState;
    fs.writeFileSync(
      path.join(runRoot, 'result.json'),
      JSON.stringify(results, null, 2),
    );
  }
  if (exitError && appPID) throw exitError;
}
