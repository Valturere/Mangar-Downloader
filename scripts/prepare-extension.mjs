import fs from 'node:fs';
import path from 'node:path';
import { packageContents } from './extension-package.mjs';
const root = path.resolve(import.meta.dirname, '..');
const target = path.resolve(process.argv[2] || '');
const allowed = path.join(root, '.local') + path.sep;
if (!target.startsWith(allowed) || fs.existsSync(target))
  throw new Error('Staging должен быть новым каталогом внутри .local');
const { files, version } = packageContents(
  path.join(root, 'browser-extension'),
);
fs.mkdirSync(target, { recursive: true });
for (const [name, content] of files) {
  const output = path.join(target, name);
  fs.mkdirSync(path.dirname(output), { recursive: true });
  fs.writeFileSync(output, content);
}
console.log(
  `Production extension ${version}: ${files.size} файлов, без dev key/diagnostics`,
);
