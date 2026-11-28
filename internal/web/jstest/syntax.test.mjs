// Every admin-page module must parse as an ES module. Node tests import only
// the pure modules, so a syntax error in a view module would otherwise pass.
import test from 'node:test';
import assert from 'node:assert/strict';
import {readdirSync, readFileSync, mkdtempSync, writeFileSync, rmSync} from 'node:fs';
import {join, dirname} from 'node:path';
import {tmpdir} from 'node:os';
import {fileURLToPath} from 'node:url';
import {spawnSync} from 'node:child_process';

const appDir = join(dirname(fileURLToPath(import.meta.url)), '..', 'assets', 'app');

test('all admin-page modules parse as ES modules', () => {
  const dir = mkdtempSync(join(tmpdir(), 'irl-syntax-'));
  try {
    for (const name of [...readdirSync(appDir).filter(n => n.endsWith('.js')), '../source.js']) {
      const copy = join(dir, name.replace('../', '').replace(/\.js$/, '.mjs'));
      writeFileSync(copy, readFileSync(join(appDir, name)));
      const r = spawnSync(process.execPath, ['--check', copy], {encoding: 'utf8'});
      assert.equal(r.status, 0, `${name}: ${r.stderr}`);
    }
  } finally {
    rmSync(dir, {recursive: true, force: true});
  }
});
