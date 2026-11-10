'use strict';

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

const source = fs.readFileSync(path.join(__dirname, 'dashboard.js'), 'utf8');
const origin = 'https://proxy.example.test';

const i18n = {
  t: (message, values = {}) => String(message).replace(/\{(\w+)\}/g, (match, key) => key in values ? (typeof values[key] === 'function' ? values[key]() : values[key]) : match),
  text(element, message, values) { element.textContent = this.t(message, values); return element; },
  attribute(element, name, message, values) { element.setAttribute(name, this.t(message, values)); },
  failure(message, values) { return new Error(this.t(message, values)); },
  error(error) { return error.message || String(error); },
  localize() {}, preference() { return 'en'; }, setLanguage() {}, refresh() {}, locale() { return 'en'; },
};

function dashboard() {
  const elements = new Map();
  const requests = [];
  const element = id => {
    if (!elements.has(id)) elements.set(id, { value: '', disabled: false });
    return elements.get(id);
  };
  vm.runInNewContext(source, {
    window: { TeamRoutingI18n: i18n, addEventListener() {} },
    document: { getElementById: element, querySelectorAll: () => [] },
    location: { origin },
    AbortSignal,
    fetch: async (url, options) => {
      requests.push(new Request(new URL(url, origin), options));
      return { ok: false, status: 503, json: async () => ({ error: 'Test response' }) };
    },
  });
  element('management-key').value = 'test-management-key';
  return { element, requests };
}

for (const [button, endpoint, method] of [
  ['connect', 'state', 'GET'],
  ['refresh', 'refresh', 'POST'],
  ['evaluate', 'evaluate', 'POST'],
]) {
  test(`${button} retains the same-origin browser session`, async () => {
    const { element, requests } = dashboard();
    await element(button).onclick();
    assert.equal(requests.length, 1);
    const request = requests[0];
    assert.equal(request.url, `${origin}/v0/management/plugins/team-routing/${endpoint}`);
    assert.equal(request.method, method);
    assert.equal(request.credentials, 'same-origin');
    assert.equal(request.headers.get('Authorization'), 'Bearer test-management-key');
    assert.equal(element('notice').textContent, 'Test response');
    assert.equal(element(button).disabled, false);
  });
}

test('connection still requires a management key', async () => {
  const { element, requests } = dashboard();
  element('management-key').value = '';
  await element('connect').onclick();
  assert.equal(requests.length, 0);
  assert.equal(element('notice').textContent, 'Enter the management key.');
});
