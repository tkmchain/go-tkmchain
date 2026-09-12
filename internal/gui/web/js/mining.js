/* Mining — RandomX mining status and control. */
(function () {
  'use strict';
  const { rpc, register, el, card, stat, table, fmtHash, fmtHexNum, live, startTicker, runAction, toast } = window.GUI;

  const section = {
    id: 'mining',
    label: 'Mining',
    icon: '⛏',
    async render(view) {
      view.appendChild(card('Mining', 'RandomX PoW'));
      view.appendChild(card('Work', 'current external-mining work tuple'));
      await live(view);
      await refreshMining(view);
      startTicker(section);
    },
    async refresh(view) {
      await refreshMining(view);
    },
  };
  register(section);

  async function etherbase() {
    try {
      const info = await rpc('admin_nodeInfo', []);
      if (info && info.coinbase && info.coinbase !== '0x' + '0'.repeat(40)) return info.coinbase;
    } catch (e) { /* fall through */ }
    return null;
  }

  async function refreshMining(view) {
    const [mining, hashrate, height] = await Promise.all([
      rpc('eth_mining', []),
      rpc('randomx_getHashrate', []).catch(() => null),
      rpc('randomx_getCurrentHeight', []).catch(() => null),
    ]);

    const statusCard = view.querySelector('.card');
    statusCard.body.innerHTML = '';
    const g = el('div', { class: 'grid' });
    g.appendChild(stat('Mining', mining ? '⛏ active' : 'idle', mining ? 'green' : 'red'));
    g.appendChild(stat('Hashrate', hashrate ? fmtHexNum(hashrate) + ' H/s' : '—'));
    g.appendChild(stat('Height', height != null ? '#' + fmtHexNum(height).toLocaleString() : '—'));
    const eb = await etherbase();
    g.appendChild(stat('Etherbase', eb ? fmtHash(eb, 16) : '—'));
    statusCard.body.appendChild(g);

    const ebInput = el('input', { class: 'txt grow', placeholder: '0x… etherbase to receive rewards' });
    const btn = el('button', { class: 'btn', text: 'Set etherbase', onclick: () => {
      const addr = ebInput.value.trim();
      if (!/^0x[0-9a-fA-F]{40}$/.test(addr)) { toast('Enter a valid 0x address.', 'err'); return; }
      runAction(btn, async () => {
        const ok = await rpc('miner_setEtherbase', [addr]);
        toast(ok ? 'Etherbase set to ' + addr : 'Etherbase not accepted', ok ? 'ok' : 'err');
      });
    } });
    statusCard.body.appendChild(el('div', { class: 'btn-row' }, [ebInput, btn]));

    const mode = el('div', { class: 'dim', style: 'margin-top:12px' });
    if (mining) {
      mode.textContent = 'Mining is active. Threads and boost are configured at node start with --mine --miner.threads=N and --randomx.boost.';
    } else {
      mode.textContent = 'Mining is not active. Start it with: gtkm gui --mine --miner.threads=2 --miner.etherbase=0xYourAddress  (add --randomx.boost for JIT+AES).';
    }
    statusCard.body.appendChild(mode);

    const workCard = view.querySelectorAll('.card')[1];
    workCard.body.innerHTML = '';
    let work;
    try { work = await rpc('miner_getWork', []); } catch (e) { work = null; }
    if (!work || !Array.isArray(work)) {
      workCard.body.appendChild(el('div', { class: 'empty', text: 'No mining work available (mining not active).' }));
      return;
    }
    const rows = [
      [el('td', { text: 'Seal hash' }), el('td', { class: 'mono', text: work[0] || '—' })],
      [el('td', { text: 'Seed hash' }), el('td', { class: 'mono', text: work[1] || '—' })],
      [el('td', { text: 'Target' }), el('td', { class: 'mono', text: work[2] || '—' })],
      [el('td', { text: 'Block height' }), el('td', { text: work[3] != null ? '#' + fmtHexNum(work[3]).toLocaleString() : '—' })],
    ];
    workCard.body.appendChild(table(['Item', 'Value'], rows));
  }
})();