/* Zero-dependency localization tests: node --test i18n.test.cjs */
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const source = fs.readFileSync(__dirname + '/i18n.js', 'utf8');

function setup({ languages = ['en-US'], saved = null, blockedStorage = false } = {}) {
  class Element {
    constructor(tag = 'div', root = false) { this.tagName = tag; this.children = []; this.parentNode = null; this.root = root; this.attributes = {}; this.value = ''; this.checked = false; }
    get isConnected() { return this.root || !!this.parentNode?.isConnected; }
    append(...children) { children.forEach(child => { child.parentNode = this; this.children.push(child); }); }
    replaceChildren(...children) { this.children.forEach(child => { child.parentNode = null; }); this.children = []; this.append(...children); }
    get textContent() { return this.children.map(child => child.nodeValue ?? child.textContent).join(''); }
    setAttribute(name, value) { this.attributes[name] = value; }
    querySelectorAll() { return []; }
  }
  const root = new Element('html', true), events = new Map(), storage = new Map(saved === null ? [] : [['team-routing.language', saved]]);
  const document = { documentElement: root, createTextNode: value => ({ nodeValue: value, parentNode: null, get isConnected() { return !!this.parentNode?.isConnected; } }) };
  const window = { addEventListener: (name, fn) => { const list = events.get(name) || []; list.push(fn); events.set(name, list); }, dispatchEvent: event => { (events.get(event.type) || []).forEach(fn => fn(event)); } };
  const context = vm.createContext({ window, document, navigator: { languages }, localStorage: { getItem: key => { if (blockedStorage) throw Error('denied'); return storage.get(key) ?? null; }, setItem: (key, value) => { if (blockedStorage) throw Error('denied'); storage.set(key, value); } }, CustomEvent: class { constructor(type) { this.type = type; } } });
  vm.runInContext(source, context);
  return { i18n: window.TeamRoutingI18n, root, Element, storage, window, context };
}

if (process.argv.includes('--metadata')) {
  const { i18n } = setup({ languages: ['zh-CN'] });
  const brands = new Set(['Claude', 'Claude / Codex', 'codex']);
  const missing = JSON.parse(fs.readFileSync(0, 'utf8')).filter(text => text && !brands.has(text) && i18n.t(text) === text);
  assert.deepEqual([...new Set(missing)], [], 'missing runtime schema/rule translations');
} else {
  const { test } = require('node:test');
  test('browser detection, explicit preference, persistence and English fallback', () => {
    assert.equal(setup({ languages: ['fr-FR', 'zh-TW', 'en'] }).i18n.locale(), 'zh-CN');
    assert.equal(setup({ languages: ['en-GB', 'zh-CN'] }).i18n.locale(), 'en');
    assert.equal(setup({ languages: ['ja'] }).i18n.locale(), 'en');
    assert.equal(setup({ languages: ['zh'], saved: 'en' }).i18n.locale(), 'en');
    const { i18n, storage } = setup({ languages: ['en'], saved: 'invalid' });
    assert.equal(i18n.preference(), 'auto');
    i18n.setLanguage('zh-CN');
    assert.equal(storage.get('team-routing.language'), 'zh-CN');
    assert.equal(i18n.t('Policy'), '策略');
    assert.equal(i18n.t('A new upstream message: gpt-example'), 'A new upstream message: gpt-example');
    i18n.setLanguage('en'); assert.equal(i18n.t('Policy'), 'Policy');
    i18n.setLanguage('auto'); assert.equal(i18n.locale(), 'en');
    assert.deepEqual([...storage.keys()], ['team-routing.language']);
  });
  test('blocked storage and browser language changes remain usable', () => {
    const { i18n, context, window } = setup({ languages: ['zh-CN'], blockedStorage: true });
    assert.doesNotThrow(() => i18n.setLanguage('en'));
    assert.equal(i18n.locale(), 'en');
    i18n.setLanguage('auto');
    context.navigator.languages = ['en-US'];
    window.dispatchEvent({ type: 'languagechange' });
    assert.equal(i18n.locale(), 'en');
  });
  test('all catalog entries preserve placeholders and translate', () => {
    const dictionary = vm.runInNewContext('(' + source.match(/const messages = (\{[\s\S]+?\n  \});/)[1] + ')');
    const placeholders = text => [...text.matchAll(/\{([a-zA-Z_]+)\}/g)].map(match => match[1]).sort();
    const { i18n } = setup({ languages: ['zh'] });
    for (const [english, chinese] of Object.entries(dictionary)) {
      assert.notEqual(english, chinese, english);
      assert.deepEqual(placeholders(english), placeholders(chinese), english);
      assert.equal(i18n.t(english), chinese, english);
    }
  });
  test('static HTML text, placeholders and accessible labels have translations', () => {
    const html = fs.readFileSync(__dirname + '/dashboard.html', 'utf8');
    const { i18n } = setup({ languages: ['zh-CN'] });
    for (const match of html.matchAll(/data-i18n(?:-aria-label|-placeholder|-title)?="([^"]+)"/g)) assert.notEqual(i18n.t(match[1]), match[1], match[1]);
    // Every English UI text leaf is marked; branding and language names are intentional.
    for (const match of html.matchAll(/<([a-z0-9]+)([^>]*)>([^<>]*[A-Za-z][^<>]*)<\//g)) {
      if (['Team Routing', 'TR', 'English'].includes(match[3])) continue;
      assert.match(match[2], /data-i18n=/, 'unmarked static text: ' + match[3]);
    }
  });
  test('language switching changes text only and keeps nested content and unfinished drafts', () => {
    const { i18n, root, Element } = setup();
    const heading = new Element('h3'), scope = new Element('span'), input = new Element('input');
    i18n.text(heading, 'Policy'); i18n.text(scope, 'Disabled'); heading.append(scope);
    input.value = 'not yet a valid number'; input.checked = true;
    i18n.attribute(input, 'aria-label', '{field} for {name}', { field: () => i18n.t('Weekly reserve'), name: 'Policy <script>{field}</script>' });
    root.append(heading, input);
    i18n.setLanguage('zh-CN');
    assert.equal(heading.textContent, '策略已禁用');
    assert.equal(heading.children[1], scope);
    assert.equal(input.value, 'not yet a valid number'); assert.equal(input.checked, true);
    assert.equal(input.attributes['aria-label'], 'Policy <script>{field}</script>：每周剩余额度预留');
    i18n.setLanguage('en'); assert.equal(heading.textContent, 'PolicyDisabled');
    i18n.text(scope, 'Available'); i18n.setLanguage('zh-CN'); assert.equal(scope.textContent, '可用');
    root.replaceChildren(input); assert.doesNotThrow(() => i18n.refresh());
  });
  test('validation notices can switch back to English without losing their source', () => {
    const { i18n, root, Element } = setup({ languages: ['zh-CN'] });
    const notice = new Element(); root.append(notice);
    const error = i18n.failure('Check {field}.', { field: () => i18n.t('Weekly reserve') });
    i18n.text(notice, i18n.error(error));
    assert.equal(notice.textContent, '请检查“每周剩余额度预留”。');
    i18n.setLanguage('en'); assert.equal(notice.textContent, 'Check Weekly reserve.');
    assert.equal(i18n.error({ name: 'TimeoutError' }), 'The request timed out. Try again later.');
  });
  test('every current routing decision code has a Chinese explanation', () => {
    const { i18n } = setup({ languages: ['zh-CN'] });
    const go = fs.readFileSync(__dirname + '/adaptive.go', 'utf8');
    for (const match of go.matchAll(/reason\s*:?=\s*"([a-z_]+)"/g)) {
      const text = match[1].replaceAll('_', ' ');
      assert.notEqual(i18n.t(text), text, match[1]);
    }
  });
  test('known backend formats preserve numbers, IDs, quoted names and unknown errors', () => {
    const { i18n } = setup({ languages: ['zh-CN'] });
    assert.equal(i18n.t('Blocked: Shared seven_day reserve and next-request buffer'), '已阻止：已触及每周额度预留及下一请求安全缓冲');
    assert.equal(i18n.t('Unlock the weekly reserve 1 hours before reset.'), '在每周额度重置前 1 小时释放预留。');
    assert.equal(i18n.t('Included five_hour quota exhausted'), '订阅内5 小时额度已耗尽');
    assert.match(i18n.t('Weekly deadline demand 2.19; 3.20 five-hour equivalents before Nov 10 23:54'), /2\.19.*Nov 10 23:54.*3\.20/);
    assert.equal(i18n.t('model "gpt-private/{model}" is not advertised by provider "custom/Claude"; reload the catalog or choose a supported pair'), '提供商 "custom/Claude" 未公布支持模型 "gpt-private/{model}"；请重新加载目录或选择支持的组合');
    assert.equal(i18n.t('{name} model rule', { name: 'Policy {name} <img>' }), 'Policy {name} <img> 的模型规则');
    assert.equal(i18n.t('Unknown provider error <script>'), 'Unknown provider error <script>');
  });
  test('application-owned dashboard and validation errors are covered', () => {
    const { i18n } = setup({ languages: ['zh-CN'] });
    for (const name of ['dashboard.go', 'account_reserves.go', 'reserves.go', 'subscription_policy.go', 'model_rules.go', 'route_evaluation.go', 'credit_guard.go']) {
      const go = fs.readFileSync(__dirname + '/' + name, 'utf8');
      for (const match of go.matchAll(/(?:fmt\.Errorf\(|"error":\s*|"message":\s*)"([^"\n]+)"/g)) {
        if (match[1].includes('%') || match[1] === 'Usage rate limited. Please try again later.') continue;
        assert.notEqual(i18n.t(match[1]), match[1], name + ': ' + match[1]);
      }
    }
  });
}
