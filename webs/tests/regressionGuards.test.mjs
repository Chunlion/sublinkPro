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

test('node import preserves hopping port lists while splitting multiple links', async () => {
  const source = await readFile(new URL('../src/views/nodes/index.jsx', import.meta.url), 'utf8');
  const block = source.match(/let nodeLinks;([\s\S]*?)if \(nodeLinks.length === 0\)/);
  assert.ok(block);
  const splitLinks = new Function('nodeForm', 'isWireGuardConfig', 'isClashYamlConfig', `let nodeLinks;${block[1]}return nodeLinks;`);
  const native = 'hysteria2://password@example.com:24443-24543,25000/?alpn=h3,h2#native';
  const legacy = 'hy2://password@[2001:db8::1]:24443/?mport=24443-24543,25000#legacy';
  const other = 'trojan://password@example.com:443#other';

  for (const separator of ['\n', '\r\n', ',', ', ']) {
    assert.deepEqual(splitLinks({ link: `${native}${separator}${legacy}${separator}${other}` }, false, false), [native, legacy, other]);
  }
  assert.deepEqual(splitLinks({ link: `  ${native}  ` }, false, false), [native]);
  const yaml = 'proxies:\n  - name: node\n    ports: 24443-24543,25000';
  assert.deepEqual(splitLinks({ link: yaml }, false, true), [yaml]);
});
