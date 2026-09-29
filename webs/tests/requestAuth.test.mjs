import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

test('permission denial keeps the session while expired authentication redirects', async () => {
  const source = await readFile(new URL('../src/api/request.js', import.meta.url), 'utf8');
  const executable = source
    .replace(/^import .*;\r?\n/gm, '')
    .replace('import.meta.env.VITE_APP_BASE_NAME', "'/'")
    .replace('export default request;', 'return request;');
  const storage = new Map([['accessToken', 'token']]);
  const localStorage = {
    getItem: (key) => storage.get(key),
    removeItem: (key) => storage.delete(key)
  };
  const window = { location: { pathname: '/shares', href: '/shares' } };
  let requestError;
  const request = new Function('axios', 'i18n', 'localStorage', 'window', 'console', executable)(
    {
      create: () => ({
        interceptors: {
          request: { use: () => {} },
          response: { use: (_, onError) => { requestError = onError; } }
        }
      })
    },
    { t: (_, fallback) => fallback },
    localStorage,
    window,
    { error: () => {} }
  );
  assert.ok(request);

  await assert.rejects(requestError({ response: { status: 403 }, config: { url: '/v1/share/delete' } }));
  assert.equal(storage.get('accessToken'), 'token');
  assert.equal(window.location.href, '/shares');

  await assert.rejects(requestError({ response: { status: 401 }, config: { url: '/v1/user/info' } }));
  assert.equal(storage.has('accessToken'), false);
  assert.equal(window.location.href, '/login');
});
