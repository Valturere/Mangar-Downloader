// Real installed Chrome/Edge, unpacked MV3, Native Messaging, production EXE.
// All profiles/media/logs are isolated. No real capture payload is printed/saved.
import { chromium } from '../frontend/node_modules/playwright/index.mjs';
import { createServer } from 'node:http';
import { createServer as netServer } from 'node:net';
import { spawn, spawnSync } from 'node:child_process';
import { randomUUID, createHash } from 'node:crypto';
import * as fs from 'node:fs';
import path from 'node:path';
import assert from 'node:assert/strict';
const channel = process.argv[2] || 'chrome';
if (!['chrome', 'msedge'].includes(channel)) throw Error('chrome или msedge');
const root = path.resolve(import.meta.dirname, '..');
const binaryDirectory =
  process.env.MANGAR_BROWSER_SMOKE_BIN || path.join(root, 'build/bin');
const runRoot = path.join(
  root,
  '.local',
  'stage7-2',
  `${channel}-${randomUUID()}`,
);
const profile = path.join(runRoot, 'profile');
const appRoot = path.join(profile, 'YTDLPGUIMangar');
const output = path.join(runRoot, 'output');
fs.mkdirSync(output, { recursive: true });
const source = path.join(process.env.LOCALAPPDATA, 'YTDLPGUIMangar', 'tools');
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
const exes = {};
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
const mediaRoot = path.join(runRoot, 'media');
fs.mkdirSync(mediaRoot);
function run(exe, args) {
  const r = spawnSync(exe, args, {
    cwd: mediaRoot,
    windowsHide: true,
    encoding: 'utf8',
    timeout: 30000,
  });
  if (r.status !== 0) throw Error('Fixture/validation process failed');
  return r.stdout;
}
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
  '-profile:v',
  'baseline',
  '-level:v',
  '1.1',
  '-pix_fmt',
  'yuv420p',
  '-c:a',
  'aac',
  '-f',
  'hls',
  '-hls_time',
  '1',
  '-hls_list_size',
  '0',
  '-hls_segment_type',
  'fmp4',
  path.join(mediaRoot, 'media.m3u8'),
]);
fs.writeFileSync(
  path.join(mediaRoot, 'master.m3u8'),
  '#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=400000,CODECS="avc1.42C00B,mp4a.40.2"\nmedia.m3u8\n',
);
const delay = (ms) => new Promise((r) => setTimeout(r, ms));
async function wait(fn, label, timeout = 30000) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    try {
      const r = await fn();
      if (r) return r;
    } catch {}
    await delay(100);
  }
  throw Error(label);
}
async function listen(server) {
  await new Promise((r) => server.listen(0, '127.0.0.1', r));
  return `http://127.0.0.1:${server.address().port}`;
}
async function freePort() {
  const s = netServer();
  await new Promise((r) => s.listen(0, '127.0.0.1', r));
  const p = s.address().port;
  await new Promise((r) => s.close(r));
  return p;
}
let frameOrigin;
const seen = {};
let denied = 0;
const mediaServer = createServer((req, res) => {
  res.setHeader('Access-Control-Allow-Origin', frameOrigin);
  res.setHeader('Access-Control-Allow-Credentials', 'true');
  res.setHeader('Access-Control-Allow-Headers', 'X-Playback');
  if (req.method === 'OPTIONS') {
    res.writeHead(204);
    res.end();
    return;
  }
  const resource = new URL(req.url, 'http://fixture.test').pathname.slice(1);
  if (
    req.headers.origin !== frameOrigin ||
    req.headers.referer !== frameOrigin + '/' ||
    req.headers['x-playback'] !== 'BROWSER_CUSTOM_FIXTURE' ||
    !req.headers.cookie?.includes('session=BROWSER_COOKIE_FIXTURE')
  ) {
    denied++;
    res.writeHead(403);
    res.end('Forbidden');
    return;
  }
  if (
    !/^[a-zA-Z0-9._-]+$/.test(resource) ||
    !fs.existsSync(path.join(mediaRoot, resource))
  ) {
    res.writeHead(404);
    res.end();
    return;
  }
  seen[resource] = (seen[resource] || 0) + 1;
  res.setHeader(
    'Content-Type',
    resource.endsWith('.m3u8') ? 'application/vnd.apple.mpegurl' : 'video/mp4',
  );
  res.end(fs.readFileSync(path.join(mediaRoot, resource)));
});
const mediaOrigin = await listen(mediaServer);
const frameServer = createServer((req, res) => {
  res.setHeader('Content-Type', 'text/html; charset=utf-8');
  res.end(`<!doctype html><title>Embed fixture</title><video muted autoplay controls></video><p id="state">Загрузка</p><script>
async function get(name){const r=await fetch('${mediaOrigin}/'+name,{headers:{'X-Playback':'BROWSER_CUSTOM_FIXTURE'},credentials:'include'});if(!r.ok)throw Error('media rejected');return r}
(async()=>{await(await get('master.m3u8')).text();const text=await(await get('media.m3u8')).text();const init=text.match(/URI="([^"]+)"/)[1];const segments=text.split('\\n').filter(l=>l&&!l.startsWith('#'));const video=document.querySelector('video');const source=new MediaSource();video.src=URL.createObjectURL(source);await new Promise(r=>source.addEventListener('sourceopen',r,{once:true}));const buffer=source.addSourceBuffer('video/mp4; codecs="avc1.42C00B,mp4a.40.2"');for(const name of [init,...segments]){const data=await(await get(name)).arrayBuffer();buffer.appendBuffer(data);await new Promise(r=>buffer.addEventListener('updateend',r,{once:true}))}source.endOfStream();await video.play();document.querySelector('#state').textContent='Воспроизводится'})().catch(e=>document.querySelector('#state').textContent='Ошибка воспроизведения: '+e.message);
</script>`);
});
frameOrigin = await listen(frameServer);
const pageServer = createServer((req, res) => {
  res.setHeader('Content-Type', 'text/html; charset=utf-8');
  res.end(
    `<!doctype html><title>Тестовое видео Mangar</title><iframe src="${frameOrigin}/embed"></iframe>`,
  );
});
const pageOrigin = await listen(pageServer);
const env = {
  ...process.env,
  LOCALAPPDATA: profile,
};
let context, media, worker, popup;
let cookieProbe;
async function nativeCookieProbe() {
  return new Promise((resolve) => {
    const p = spawn(
      exes['yt-dlp.exe'],
      [
        '--ignore-config',
        '--no-plugin-dirs',
        '--no-remote-components',
        '--cookies-from-browser',
        channel === 'msedge' ? 'edge' : 'chrome',
        '--list-impersonate-targets',
      ],
      { env, windowsHide: true },
    );
    let text = '';
    const timer = setTimeout(() => p.kill(), 15000);
    for (const stream of [p.stdout, p.stderr])
      stream.on('data', (d) => {
        if (text.length < 64000) text += d.toString();
      });
    p.on('close', (code) => {
      clearTimeout(timer);
      resolve({
        code,
        extracted: /Extracted \d+ cookies/.test(text),
        accessFailure:
          /could not copy|failed to decrypt|failed to load cookies|database is locked|could not find/i.test(
            text,
          ),
      });
    });
  });
}
function native(action = 'Snapshot') {
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
      path.join(binaryDirectory, 'YTDLP-GUI-Mangar.exe'),
    ],
    { windowsHide: true, encoding: 'utf8', timeout: 30000 },
  );
  if (r.status !== 0) throw Error('Native smoke action failed: ' + action);
  return JSON.parse(r.stdout.trim());
}
function completedFiles() {
  return fs.readdirSync(output).filter((n) => n.endsWith('.mp4'));
}
async function actionPopup(cdp, id, targetId) {
  const previous = await cdp.send('Target.getTargets', {
    filter: [{ exclude: false }],
  });
  for (const old of previous.targetInfos.filter((t) =>
    t.url.endsWith('/popup.html'),
  ))
    await cdp
      .send('Target.closeTarget', { targetId: old.targetId })
      .catch(() => {});
  await cdp.send('Extensions.triggerAction', { id, targetId });
  const target = await wait(async () => {
    const { targetInfos } = await cdp.send('Target.getTargets', {
      filter: [{ exclude: false }],
    });
    return targetInfos.find((t) => t.url.endsWith('/popup.html'));
  }, 'Popup target absent');
  const { sessionId } = await cdp.send('Target.attachToTarget', {
    targetId: target.targetId,
    flatten: false,
  });
  let sequence = 0;
  const evaluate = (expression) =>
    new Promise((resolve, reject) => {
      const messageId = ++sequence;
      const timeout = setTimeout(() => {
        cdp.off('Target.receivedMessageFromTarget', receive);
        reject(Error('Popup response timeout'));
      }, 10000);
      function receive(e) {
        if (e.sessionId !== sessionId) return;
        const m = JSON.parse(e.message);
        if (m.id !== messageId) return;
        clearTimeout(timeout);
        cdp.off('Target.receivedMessageFromTarget', receive);
        if (m.error || m.result.exceptionDetails)
          reject(Error('Popup evaluation failed'));
        else resolve(m.result.result.value);
      }
      cdp.on('Target.receivedMessageFromTarget', receive);
      cdp
        .send('Target.sendMessageToTarget', {
          sessionId,
          message: JSON.stringify({
            id: messageId,
            method: 'Runtime.evaluate',
            params: { expression, returnByValue: true, awaitPromise: true },
          }),
        })
        .catch(reject);
    });
  await wait(
    () => evaluate('document.querySelectorAll("article").length'),
    'Popup candidates absent',
  );
  const text = await evaluate('document.body.innerText');
  assert(!text.includes('BROWSER_COOKIE_FIXTURE'));
  assert(!text.includes('token='));
  await evaluate(
    'Array.from(document.querySelectorAll("article")).find(x=>x.innerText.includes("HLS")).querySelector("button").click();true',
  );
  return {
    evaluate,
    close: () =>
      cdp
        .send('Target.closeTarget', { targetId: target.targetId })
        .catch(() => {}),
  };
}
async function stablePopup(context, media, id, host = '') {
  // Keep the real extension document alive across OS foreground transitions.
  // The toolbar popup itself was exercised by the cold-start capture above.
  const p = await context.newPage();
  await p.goto(`chrome-extension://${id}/popup.html`);
  await media.bringToFront();
  await p.reload();
  await p.locator('article').first().waitFor();
  const button = p
    .locator('article')
    .filter({ hasText: host || 'HLS' })
    .first()
    .locator('button');
  await button.evaluate((b) => b.click());
  await wait(
    () =>
      p
        .locator('#status')
        .textContent()
        .then((s) => s && s !== 'Передача…'),
    'Stable popup reply',
    20000,
  );
  const message = await p.locator('#status').textContent();
  assert.equal(message, 'Передано в Mangar');
  return {
    evaluate: (expression) => p.evaluate(expression),
    close: () => p.close(),
  };
}
try {
  context = await chromium.launchPersistentContext(
    path.join(
      profile,
      channel === 'msedge'
        ? 'Microsoft/Edge/User Data'
        : 'Google/Chrome/User Data',
    ),
    {
      channel,
      headless: false,
      ignoreDefaultArgs: ['--disable-extensions'],
      env,
      args: ['--enable-unsafe-extension-debugging'],
    },
  );
  const cdp = await context.browser().newBrowserCDPSession();
  const { id } = await cdp.send('Extensions.loadUnpacked', {
    path: path.join(root, 'browser-extension'),
  });
  worker = await wait(
    () => context.serviceWorkers().find((w) => w.url().includes(id)),
    'Extension worker startup',
  );
  await context.addCookies([
    {
      name: 'session',
      value: 'BROWSER_COOKIE_FIXTURE',
      url: mediaOrigin,
      httpOnly: true,
      sameSite: 'Lax',
    },
  ]);
  cookieProbe = await nativeCookieProbe();
  media = await context.newPage();
  media.on('pageerror', (e) => console.log('fixture page error: ' + e.message));
  await media.goto(pageOrigin);
  await wait(
    () =>
      media
        .frameLocator('iframe')
        .locator('#state')
        .textContent()
        .then((x) => x === 'Воспроизводится'),
    'HLS browser playback failed',
  );
  worker = await wait(
    () => context.serviceWorkers().find((w) => w.url().includes(id)),
    'Extension worker absent',
  );
  await wait(
    () =>
      worker.evaluate(async () => {
        const { captures = [] } = await chrome.storage.session.get('captures');
        return captures.some(
          (c) =>
            c.browser.mediaType === 'hls' &&
            c.frameId > 0 &&
            c.browser.cookies.length === 1 &&
            c.browser.exactCookies,
        );
      }),
    'Captured iframe/cookies missing',
  );
  // Actual action popup; its messages retain the media tab as active.
  const { targetInfos } = await cdp.send('Target.getTargets', {
    filter: [{ type: 'tab', exclude: false }, { exclude: true }],
  });
  const targetInfo = targetInfos.find((t) => t.url === media.url());
  popup = await actionPopup(cdp, id, targetInfo.targetId);
  // App was closed: helper starts it, waits for DOM, preserves the capture.
  const initial = await wait(() => {
    const s = native();
    return s.visible &&
      s.tray &&
      s.names.includes(
        'Видео получено из браузера. Выберите папку для загрузки.',
      )
      ? s
      : false;
  }, 'Native Mangar/folder prompt absent');
  assert(initial.visible && initial.windowCount === 1 && initial.tray);
  assert(
    initial.names.includes(
      'Видео получено из браузера. Выберите папку для загрузки.',
    ),
  );
  assert(!JSON.stringify(initial).includes('BROWSER_COOKIE_FIXTURE'));
  native('Folder');
  await wait(
    () => completedFiles().length === 1,
    'Browser job did not complete',
    60000,
  );
  for (const file of completedFiles()) {
    const probe = JSON.parse(
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
    assert(probe.format.format_name.includes('mp4'));
    assert(probe.streams.some((s) => s.codec_type === 'video'));
    assert(probe.streams.some((s) => s.codec_type === 'audio'));
  }
  // Second manual launch uses same endpoint; second process must exit.
  native('Hide');
  await wait(() => !native().visible, 'Initial hide');
  const second = spawn(path.join(binaryDirectory, 'YTDLP-GUI-Mangar.exe'), [], {
    env,
    windowsHide: true,
    stdio: 'ignore',
  });
  await wait(
    () => second.exitCode !== null,
    'Second instance did not exit',
    15000,
  );
  assert.equal(second.exitCode, 0);
  await wait(
    () => native().visible,
    'Second launch did not show original window',
  );
  // Hide existing app, make a fresh browser request, then send another capture.
  const shown = native();
  assert.equal(shown.pid, initial.pid);
  assert.equal(shown.windowCount, 1);
  native('Hide');
  await wait(() => !native().visible, 'Tray hide');
  await media.reload();
  await wait(
    () =>
      media
        .frameLocator('iframe')
        .locator('#state')
        .textContent()
        .then((x) => x === 'Воспроизводится'),
    'Reload playback',
  );
  await popup.close();
  await media.bringToFront();
  popup = await stablePopup(context, media, id);
  await wait(
    () => completedFiles().length === 2,
    'Tray job did not complete',
    60000,
  );
  const hidden = native();
  assert.equal(hidden.pid, initial.pid);
  assert.equal(hidden.visible, false);
  assert(
    seen['master.m3u8'] > 0 &&
      seen['media.m3u8'] > 0 &&
      seen['init.mp4'] > 0 &&
      Object.keys(seen).some((p) => p.endsWith('.m4s')),
  );
  const temp = path.join(appRoot, 'temp');
  assert(!fs.existsSync(temp) || fs.readdirSync(temp).length === 0);
  const logs = fs
    .readdirSync(path.join(appRoot, 'logs/jobs'))
    .map((n) => fs.readFileSync(path.join(appRoot, 'logs/jobs', n), 'utf8'))
    .join('');
  assert(!logs.includes('BROWSER_COOKIE_FIXTURE'));
  assert(!logs.includes('BROWSER_CUSTOM_FIXTURE'));
  let vkResult;
  if (process.env.MANGAR_VK_PAGE) {
    await popup.close();
    const vk = await context.newPage();
    await vk.goto(process.env.MANGAR_VK_PAGE, {
      waitUntil: 'domcontentloaded',
    });
    await vk
      .getByText('Плеер 2', { exact: true })
      .first()
      .click({ timeout: 15000 });
    const embed = await wait(
      () =>
        vk.frames().find((f) => {
          try {
            return new URL(f.url()).hostname === 'alibi.stravers.live';
          } catch {
            return false;
          }
        }),
      'VK iframe absent',
    );
    await embed.locator('video').first().click({ timeout: 15000 });
    await embed.evaluate(async () => {
      for (const video of document.querySelectorAll('video'))
        await video.play().catch(() => {});
    });
    const currentWorker = await wait(
      () => context.serviceWorkers().find((w) => w.url().includes(id)),
      'VK worker',
    );
    const candidate = await wait(
      () =>
        currentWorker.evaluate(async () => {
          const { captures = [] } =
            await chrome.storage.session.get('captures');
          const c = captures.find(
            (c) =>
              c.browser.mediaType === 'hls' &&
              new URL(c.url).hostname.endsWith('.vkvideo.cloud'),
          );
          return c
            ? {
                host: new URL(c.url).hostname,
                frame: c.frameId,
                cookies: c.browser.cookies.length,
                exact: c.browser.exactCookies,
                referer: !!c.http.referer,
                origin: !!c.http.origin,
                userAgent: !!c.http.userAgent,
                headerNames: Object.keys(c.http.headers),
              }
            : null;
        }),
      'VK browser manifest absent',
      60000,
    );
    let browserPlaying = true;
    await wait(
      () =>
        embed.evaluate(() =>
          Array.from(document.querySelectorAll('video')).some(
            (v) => !v.paused && v.currentTime > 0 && v.readyState >= 2,
          ),
        ),
      'VK playback did not start',
      20000,
    ).catch(() => {
      browserPlaying = false;
    });
    console.log(
      JSON.stringify({
        vkCapture: candidate,
        browserPlaying,
        videoState: await embed.evaluate(() =>
          Array.from(document.querySelectorAll('video')).map((v) => ({
            paused: v.paused,
            time: v.currentTime,
            ready: v.readyState,
            error: v.error?.code,
            duration: Number.isFinite(v.duration) ? v.duration : null,
          })),
        ),
      }),
    );
    popup = await stablePopup(context, vk, id, candidate.host);
    console.log('VK capture accepted; waiting for MP4');
    const started = Date.now();
    let last = 0;
    function vkLog() {
      const dir = path.join(appRoot, 'logs/jobs');
      const files = fs
        .readdirSync(dir)
        .map((n) => ({ n, time: fs.statSync(path.join(dir, n)).mtimeMs }))
        .sort((a, b) => b.time - a.time);
      return files.length
        ? fs.readFileSync(path.join(dir, files[0].n), 'utf8')
        : '';
    }
    await wait(
      () => {
        if (completedFiles().length === 3) return true;
        if (vkLog().includes('"phase":"finish"')) return true;
        if (Date.now() - last > 30000) {
          last = Date.now();
          console.log(
            'VK download elapsed seconds=' +
              Math.round((last - started) / 1000),
          );
        }
        return false;
      },
      'VK did not complete',
      600000,
    );
    if (completedFiles().length !== 3) {
      const log = vkLog();
      console.log(
        JSON.stringify({
          vkFailed: true,
          http403: /HTTP(?: Error)?\s+403/.test(log),
          http401: /HTTP(?: Error)?\s+401/.test(log),
          missingFragment:
            /No such file or directory.*media\.mp4\.part-Frag/.test(log),
          liveWarning: log.includes('Live HLS streams'),
          impersonation: log.includes('"stream":"impersonation"'),
          cookieAbsent: candidate.cookies === 0,
          tempClean:
            !fs.existsSync(path.join(appRoot, 'temp')) ||
            !fs.readdirSync(path.join(appRoot, 'temp')).length,
        }),
      );
      throw Error('VK engine failed; see isolated redacted technical log');
    }
    const file = completedFiles().find(
      (n) => !n.startsWith('Тестовое видео Mangar'),
    );
    const probe = JSON.parse(
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
    assert(probe.format.format_name.includes('mp4'));
    assert(
      probe.streams.some((s) => s.codec_type === 'video') &&
        probe.streams.some((s) => s.codec_type === 'audio'),
    );
    vkResult = {
      completed: true,
      duration: probe.format.duration,
      streams: probe.streams.map((s) => ({
        type: s.codec_type,
        codec: s.codec_name,
        width: s.width,
        height: s.height,
      })),
      candidate,
    };
  }
  console.log(
    JSON.stringify({
      channel,
      version: context.browser().version(),
      iframe: true,
      exactCookies: true,
      appClosed: true,
      secondLaunch: true,
      tray: true,
      mp4: 2,
      seen,
      denied,
      cookieProbe,
      vkResult,
      artifacts: runRoot,
    }),
  );
} catch (e) {
  try {
    console.log('Native diagnostics: ' + JSON.stringify(native()));
  } catch (error) {
    console.log(error.message);
  }
  if (popup)
    console.log(
      'Popup result: ' +
        (await popup
          .evaluate('document.querySelector("#status").textContent')
          .catch(() => 'closed')),
    );
  if (worker)
    console.log(
      'Capture diagnostics: ' +
        JSON.stringify(
          await worker
            .evaluate(async () => {
              const { captures = [] } =
                await chrome.storage.session.get('captures');
              return captures.map((c) => ({
                type: c.browser.mediaType,
                frame: c.frameId,
                headerNames: Object.keys(c.http.headers),
                cookies: c.browser.cookies.length,
                exact: c.browser.exactCookies,
                incomplete: c.cookieIncomplete,
                auth: c.authorizationRequired,
              }));
            })
            .catch(() => []),
        ),
    );
  if (media)
    console.log(
      'fixture state: ' +
        (await media
          .frameLocator('iframe')
          .locator('#state')
          .textContent()
          .catch(() => '?')),
    );
  console.error(
    `FAIL ${channel}: ${e.message.split('\n')[0]}; media=${JSON.stringify(seen)} denied=${denied}; artifacts=${runRoot}`,
  );
  process.exitCode = 1;
} finally {
  try {
    native('Exit');
  } catch {}
  if (context) await context.close();
  for (const server of [pageServer, frameServer, mediaServer])
    await new Promise((r) => server.close(r));
  assert.equal(fingerprint(), before, 'Production tools changed');
}
