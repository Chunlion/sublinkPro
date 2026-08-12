import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

test('template stream login redirect respects the configured base path', async () => {
  const source = await readFile(new URL('../src/api/templates.js', import.meta.url), 'utf8');

  assert.match(source, /__SUBLINK_CONFIG__\?\.basePath/);
  assert.doesNotMatch(source, /window\.location\.href = ['"]\/login['"]/);
});

test('share list ignores stale request results', async () => {
  const source = await readFile(new URL('../src/views/subscriptions/component/ShareManageDialog.jsx', import.meta.url), 'utf8');

  assert.match(source, /const requestId = \+\+listRequestIdRef\.current/);
  assert.match(source, /requestId !== listRequestIdRef\.current/);
});
