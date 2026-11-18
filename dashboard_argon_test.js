'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const test = require('node:test');

test('quota cells label Argon separately and preserve a decimal; disabled buckets have no meter', () => {
  const source = fs.readFileSync(__dirname + '/dashboard.js', 'utf8');
  const start = source.indexOf('  function bucketCell('), end = source.indexOf('  function accountDetails(', start);
  function el(tag, text, className) { return { tag, text, className, children: [], style: {}, append(...nodes) { this.children.push(...nodes); } }; }
  const cell = vm.runInNewContext(source.slice(start, end) + '\nbucketCell(buckets)', {
    el, tr: el, percent: v => (v * 100).toFixed(0) + '%', date: v => v,
    buckets: [{ groupName: 'Gemini Models', remainingFraction: .9999736, Window: 'weekly' }, { groupName: 'Gemini 4 Argon', remainingFraction: .876176, Window: 'weekly' }, { groupName: 'Disabled pool', disabled: true, remainingFraction: 1 }],
  });
  const serialized = JSON.stringify(cell);
  assert.match(serialized, /Gemini 4 Argon/);
  assert.match(serialized, /87\.6%/);
  assert.match(serialized, /Gemini Models/);
  assert.equal((serialized.match(/"className":"meter"/g) || []).length, 2);
  assert.match(serialized, /Disabled/);
});
