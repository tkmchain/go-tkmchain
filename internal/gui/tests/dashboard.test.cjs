const {test} = require('node:test');
const assert = require('node:assert/strict');
const {readFileSync} = require('node:fs');
const vm = require('node:vm');

for (const mode of [false, {currentBlock:'0x5', highestBlock:'0xa'}, 'offline']) {
  test(`dashboard renders and refreshes: ${JSON.stringify(mode)}`, async () => {
    const node = (text = '') => ({text, children: [], appendChild(child) {this.children.push(child); return child;}, set innerHTML(_) {this.children = [];}});
    const view = node(); view.querySelectorAll = () => view.children;
    let section, ticker, calls = 0;
    const GUI = {
      register(s) {section = s;},
      async rpc(method) {calls++; if(mode === 'offline') throw Error('unavailable'); return method === 'admin_peers' ? [] : method === 'eth_getBlockByNumber' ? null : '0x1';},
      async rpcBatch() {return ['0x2313', mode, '0xa'];},
      el: (_, attrs = {}) => node(attrs.text),
      card() {const n = node(); n.body = node(); n.appendChild(n.body); return n;},
      stat: (label, value) => node(`${label}: ${value}`), table: () => node(), empty: node,
      fmtTKM: String, fmtHash: String, fmtHexNum: v => Number(v || 0), fmtTS: String,
      async live() {}, startTicker(s) {ticker = s;},
    };
    vm.runInNewContext(readFileSync(require.resolve('../web/js/dashboard.js'), 'utf8'), {window: {GUI}});
    await section.render(view);
    assert.equal(view.children.length, 5);
    assert.equal(ticker, section);
    const text = n => [n.text, ...n.children.map(text)].join(' ');
    assert.match(text(view.children[2]), mode === false ? /Fully synced/ : mode === 'offline' ? /unavailable/ : /50%/);
    const before = calls;
    await section.refresh(view);
    assert.ok(calls > before);
    assert.equal(view.children.length, 5);
  });
}
