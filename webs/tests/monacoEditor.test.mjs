import assert from 'node:assert/strict';
import { access, readFile } from 'node:fs/promises';
import test from 'node:test';

const readProjectFile = (path) => readFile(new URL(`../${path}`, import.meta.url), 'utf8');
const monacoModules = [
  'monaco-editor/editor/editor.api',
  'monaco-editor/languages/definitions/ini/register',
  'monaco-editor/languages/definitions/javascript/register',
  'monaco-editor/languages/definitions/yaml/register',
  'monaco-editor/language/typescript/monaco.contribution',
  'monaco-editor/editor/editor.worker',
  'monaco-editor/language/typescript/ts.worker'
];

test('editor pages use the bundled Monaco configuration', async () => {
  const [templatesView, scriptsView, monacoConfig, viteConfig] = await Promise.all([
    readProjectFile('src/views/templates/index.jsx'),
    readProjectFile('src/views/scripts/index.jsx'),
    readProjectFile('src/utils/monacoEditor.js'),
    readProjectFile('vite.config.mjs')
  ]);

  assert.match(templatesView, /from 'utils\/monacoEditor'/);
  assert.match(scriptsView, /from 'utils\/monacoEditor'/);
  assert.doesNotMatch(templatesView, /from '@monaco-editor\/react'/);
  assert.doesNotMatch(scriptsView, /from '@monaco-editor\/react'/);

  assert.match(monacoConfig, /from 'monaco-editor\/editor\/editor\.api'/);
  assert.match(monacoConfig, /editor\.worker\?worker/);
  assert.match(monacoConfig, /ts\.worker\?worker/);
  assert.match(monacoConfig, /loader\.config\(\{ monaco \}\)/);
  assert.doesNotMatch(monacoConfig, /monaco-editor\/esm\/vs\//);
  assert.doesNotMatch(monacoConfig, /cdn\.jsdelivr\.net/);

  assert.match(viteConfig, /globIgnores: \['\*\*\/ts\.worker-\*\.js'\]/);
});

test('bundled Monaco modules resolve through package exports', async () => {
  await Promise.all(
    monacoModules.map(async (moduleName) => {
      const resolvedModule = import.meta.resolve(moduleName);
      await access(new URL(resolvedModule));
    })
  );
});
