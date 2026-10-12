// Production packaging leaves the opt-in Stage 7.2.2 development source intact.
import fs from 'node:fs';
import path from 'node:path';

export const runtimeFiles = [
  'manifest.json',
  'worker.js',
  'capture.js',
  'page.js',
  'format.js',
  'popup-model.js',
  'popup-view.js',
  'popup.js',
  'popup.html',
  'popup.css',
  'icons/16.png',
  'icons/32.png',
  'icons/48.png',
  'icons/128.png',
];

function unique(source, text) {
  const at = source.indexOf(text);
  if (at < 0 || source.indexOf(text, at + text.length) >= 0)
    throw new Error(
      'Production worker: структура исходника изменилась; требуется review упаковки',
    );
  return at;
}
function remove(source, text) {
  const at = unique(source, text);
  return source.slice(0, at) + source.slice(at + text.length);
}
function removeUntil(source, start, end) {
  const from = unique(source, start),
    to = unique(source, end);
  if (from >= to) throw new Error('Production worker: неверный порядок блоков');
  return source.slice(0, from) + source.slice(to);
}

export function productionWorker(input) {
  let source = input.replaceAll('\r\n', '\n');
  source = remove(
    source,
    "import { FragmentDiagnostic, DIAGNOSTIC_TTL } from './fragment-diagnostic.js';\n",
  );
  source = removeUntil(
    source,
    'const diagnostic = new FragmentDiagnostic();',
    "const HOST = 'ru.mangar.browser';",
  );
  source = removeUntil(
    source,
    'chrome.webRequest.onBeforeRequest.addListener(',
    'const pending = new Map();',
  );
  source = remove(
    source,
    '  diagnosticRevision++;\n  if (diagnostic.selection?.tabId === tabId || diagnosticOwner?.tabId === tabId)\n    stopDiagnostic();\n',
  );
  source = remove(source, '  diagnosticRevision++;\n');
  source = remove(
    source,
    '    if (\n      diagnosticOwner?.tabId === d.tabId &&\n      diagnosticOwner.frameId === d.frameId\n    )\n      stopDiagnostic();\n    diagnostic.clearTab(d.tabId, d.frameId);\n',
  );
  source = removeUntil(
    source,
    "  if (\n    sender.id === chrome.runtime.id &&\n    sender.url === chrome.runtime.getURL('diagnostic.html')",
    '  // No content scripts, external messaging or page-origin authority.',
  );
  if (/diagnostic|connectNative|FragmentDiagnostic/.test(source))
    throw new Error('Production worker: development-код остался в пакете');
  return source;
}

export function packageContents(directory) {
  const manifest = JSON.parse(
    fs.readFileSync(path.join(directory, 'manifest.json'), 'utf8'),
  );
  const parts = manifest.version?.split('.');
  if (
    !parts ||
    parts.length > 4 ||
    parts.length < 1 ||
    parts.some((v) => !/^(0|[1-9][0-9]*)$/.test(v) || Number(v) > 65535) ||
    parts.every((v) => Number(v) === 0)
  )
    throw new Error('Некорректная numeric version Chrome');
  if (
    manifest.manifest_version !== 3 ||
    manifest.name !== 'Mangar Downloader' ||
    manifest.background?.service_worker !== 'worker.js' ||
    manifest.background.type !== 'module' ||
    manifest.action?.default_popup !== 'popup.html' ||
    manifest.minimum_chrome_version !== '132'
  )
    throw new Error('Неожиданная конфигурация расширения');
  const permissions = [
    'webRequest',
    'cookies',
    'storage',
    'tabs',
    'webNavigation',
    'alarms',
    'nativeMessaging',
  ];
  if (
    JSON.stringify(manifest.permissions) !== JSON.stringify(permissions) ||
    JSON.stringify(manifest.host_permissions) !==
      JSON.stringify(['http://*/*', 'https://*/*']) ||
    manifest.content_scripts ||
    manifest.externally_connectable ||
    manifest.update_url
  )
    throw new Error(
      'Permissions/distribution изменились; требуется отдельный audit',
    );
  // A CWS item assigns its own identity. The development key is not an identity claim.
  delete manifest.key;
  manifest.content_security_policy = {
    extension_pages: "script-src 'self'; object-src 'self'",
  };
  const files = new Map(
    runtimeFiles.map((name) => {
      const content = fs.readFileSync(path.join(directory, name));
      return [
        name,
        name.endsWith('.png')
          ? content
          : Buffer.from(content.toString('utf8').replaceAll('\r\n', '\n')),
      ];
    }),
  );
  for (const size of [16, 32, 48, 128]) {
    if (
      manifest.icons?.[size] !== `icons/${size}.png` ||
      manifest.action.default_icon?.[size] !== `icons/${size}.png`
    )
      throw new Error('Не хватает extension/action icons');
    const png = files.get(`icons/${size}.png`);
    if (
      png.subarray(0, 8).toString('hex') !== '89504e470d0a1a0a' ||
      png.readUInt32BE(16) !== size ||
      png.readUInt32BE(20) !== size
    )
      throw new Error('Некорректный PNG icon');
  }
  files.set(
    'manifest.json',
    Buffer.from(JSON.stringify(manifest, null, 2) + '\n'),
  );
  files.set(
    'worker.js',
    Buffer.from(productionWorker(files.get('worker.js').toString('utf8'))),
  );
  return { version: manifest.version, files };
}
