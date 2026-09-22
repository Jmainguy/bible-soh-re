const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const source = fs.readFileSync('static/app.js', 'utf8');
const context = vm.createContext({});
vm.runInContext(source.slice(source.indexOf('async function readAPIResponse'), source.indexOf('let chapterRequestID')), context);
test('chapter errors preserve plain text and JSON server messages', async () => {
  for (const body of ['translation not found: nasb', '{"error":"Translation unavailable"}']) {
    await assert.rejects(context.readAPIResponse(new Response(body, { status: 400 })), /translation not found|Translation unavailable/);
  }
});
test('successful chapter response remains JSON', async () => {
  const result = await context.readAPIResponse(new Response('{"verses":[{"verse":1,"text":"Example"}]}'));
  assert.equal(result.verses[0].text, 'Example');
});

context.escapeHtml = text => text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;');
vm.runInContext(source.slice(source.indexOf('function formatVerseText'), source.indexOf('// Navigate to a specific book')), context);
test('scripture markers and red letters survive quote escaping', () => {
  const text = 'And they seized him and<sup class="footnote">1</sup> <sup class="cross-ref">2</sup><span class="red-letter"><i>Listen</i></span>';
  assert.equal(context.formatVerseText(text, 3), text);
});
test('verse formatting keeps ordinary text escaped and rejects unsafe markup', () => {
  assert.equal(context.formatVerseText('A & B <sup class="footnote" onclick="alert(1)">1</sup><script>alert(1)</script>', 1), 'A &amp; B <sup class="footnote">1</sup>&lt;script&gt;alert(1)&lt;/script&gt;');
});

test('every visible display option works without the removed Strong’s checkbox', () => {
  const names = ['Footnotes', 'Scripref', 'Headings', 'RedLetters', 'Lemma', 'Morph', 'Xlit'];
  const state = { currentUserId: null, osisFilters: {}, filterStrongsCheckbox: null, loadChapter: () => state.loads++, loads: 0 };
  for (const name of names) state[`filter${name}Checkbox`] = { checked: true };
  const ctx = vm.createContext(state);
  vm.runInContext(source.slice(source.indexOf('function handleFilterChange()'), source.indexOf('// Load user groups for verse comments dropdown', source.indexOf('function handleFilterChange()'))), ctx);
  for (const name of names) {
    state[`filter${name}Checkbox`].checked = false;
    ctx.handleFilterChange();
    assert.equal(state.osisFilters[`show${name}`], false);
    state[`filter${name}Checkbox`].checked = true;
    ctx.handleFilterChange();
    assert.equal(state.osisFilters[`show${name}`], true);
  }
  assert.equal(state.loads, 14);
});
