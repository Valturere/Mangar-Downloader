// Export reviewed, real Chrome screenshots. Never fabricate popup state here.
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { createHash } from 'node:crypto';
import { packageContents } from './extension-package.mjs';

export const screenshotNames = [
  '01-supported-page.png',
  '02-detected-media.png',
  '03-privacy-info.png',
];

export function validateScreenshot(data) {
  if (
    data.length < 33 ||
    data.subarray(0, 8).toString('hex') !== '89504e470d0a1a0a' ||
    data.toString('ascii', 12, 16) !== 'IHDR'
  )
    throw new Error('Store screenshot должен быть PNG');
  const width = data.readUInt32BE(16);
  const height = data.readUInt32BE(20);
  if (!(
    (width === 640 && height === 400) ||
    (width === 1280 && height === 800)
  ))
    throw new Error('CWS screenshot должен иметь размер 640×400 или 1280×800');
  return { width, height };
}

export function storeAssets(root) {
  const { files, version } = packageContents(
    path.join(root, 'browser-extension'),
  );
  const screenshots = screenshotNames.map((name) => {
    const data = fs.readFileSync(path.join(root, 'docs/cws', version, name));
    validateScreenshot(data);
    return [name, data];
  });
  return {
    version,
    assets: new Map([
      ['icon-128.png', files.get('icons/128.png')],
      ...screenshots,
    ]),
  };
}

if (
  process.argv[1] &&
  path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)
) {
  const root = path.resolve(import.meta.dirname, '..');
  const { version, assets } = storeAssets(root);
  const output = path.join(root, 'dist/release', version, 'store');
  fs.mkdirSync(output, { recursive: true });
  const records = [];
  for (const [name, data] of assets) {
    fs.writeFileSync(path.join(output, name), data);
    records.push({
      filename: name,
      size: data.length,
      sha256: createHash('sha256').update(data).digest('hex'),
      ...(screenshotNames.includes(name) ? validateScreenshot(data) : {}),
    });
  }
  fs.writeFileSync(
    path.join(output, 'store-assets.json'),
    JSON.stringify({ extension_version: version, assets: records }, null, 2) +
      '\n',
  );
  console.log(
    `PASS: 3 reviewed Chrome screenshots and packaged icon: ${output}`,
  );
}
