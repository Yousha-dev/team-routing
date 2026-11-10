'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const test = require('node:test');
const vm = require('node:vm');

class Element {
  constructor(tag) { this.tagName = tag; this.children = []; this.attributes = {}; this.value = ''; }
  append(...children) { this.children.push(...children); }
  replaceChildren(...children) { this.children = children; }
  setAttribute(key, value) { this.attributes[key] = value; }
  get dataset() { return this.data || (this.data = {}); }
}

function editor(policy, previous) {
  const context = {window: {}, document: {createElement: tag => new Element(tag)}};
  vm.runInNewContext(fs.readFileSync(require.resolve('./editors.js'), 'utf8'), context);
  const providerRoot = new Element('div'), accountRoot = new Element('div');
  const spec = {
    account_reserves: {title: 'Accounts and reserves', description: '', fields: []},
    account_priority_field: {type: 'integer', minimum: -1000000, maximum: 1000000, title: 'Account priority', description: ''},
    provider_fields: [], provider_default: {}, window_fields: []
  };
  const result = context.window.TeamRoutingEditors.subscriptionEditor(providerRoot, accountRoot,
    {accounts: [{ID: 'credential-a', Name: 'Credential A'}], catalog: {providers: []}}, policy, spec, () => {}, previous);
  const controls = [];
  function visit(node) {
    if (node.attributes?.['aria-label']?.startsWith('Account priority for ')) controls.push(node);
    for (const child of node.children || []) visit(child);
  }
  visit(accountRoot);
  return {result, controls};
}

const plain = value => JSON.parse(JSON.stringify(value));

test('priority editor saves integers, clears zero and retains disconnected rules', () => {
  const {result, controls} = editor({account_priority: {'credential-a': 10, 'credential-b': -5}});
  assert.equal(controls.length, 2);
  const first = controls.find(control => control.attributes['aria-label'].endsWith('for Credential A'));
  const disconnected = controls.find(control => control.attributes['aria-label'].endsWith('for credential-b'));
  assert.deepEqual(plain(result.read().account_priority), {'credential-a': 10, 'credential-b': -5});
  first.value = '0';
  assert.deepEqual(plain(result.read().account_priority), {'credential-b': -5});
  disconnected.value = '1000000';
  assert.deepEqual(plain(result.read().account_priority), {'credential-b': 1000000});
});

test('priority editor defaults to zero and retains unsaved drafts across refresh', () => {
  const {result, controls} = editor({});
  assert.deepEqual(plain(result.read().account_priority), {});
  controls[0].value = '-20';
  const refreshed = editor({}, result.draft());
  assert.deepEqual(plain(refreshed.result.read().account_priority), {'credential-a': -20});
});

test('priority editor rejects empty, non-integer and out-of-range input', () => {
  const {result, controls} = editor({});
  for (const invalid of ['', '1.5', 'NaN', '1000001', '-1000001']) {
    controls[0].value = invalid;
    assert.throws(() => result.read(), /Check Account priority/);
  }
});
