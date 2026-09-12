/* Supply — tkmsupply accounting index. */
(function () {
  'use strict';
  const { rpc, register, el, card, stat, fmtHexNum, fmtTKM, live, runAction, toast, jsonView } = window.GUI;

  const section = {
    id: 'supply',
    label: 'Supply',
    icon: 'Σ',
    async render(view) {
      view.appendChild(card('Latest supply', 'tkmsupply_latest'));
      view.appendChild(card('Historical', 'tkmsupply_atBlock'));
      view.appendChild(card('Sync index', 'tkmsupply_sync'));
      view.appendChild(card('Raw', ''));
      await live(view);
      await refreshSupply(view);
    },
    async refresh(view) {
      await refreshSupply(view);
    },
  };
  register(section);

  async function refreshSupply(view) {
    const cards = view.querySelectorAll('.card');
    const [latestEl, histEl, syncEl, rawEl] = cards;

    latestEl.body.innerHTML = '';
    rawEl.body.innerHTML = '';
    try {
      const latest = await rpc('tkmsupply_latest', []);
      rawEl.body.appendChild(jsonView(latest));
      if (!latest) { latestEl.body.appendChild(window.GUI.empty('No supply data yet.')); return; }
      const g = el('div', { class: 'grid' });
      g.appendChild(stat('Height', '#' + fmtHexNum(latest.height ?? latest.block).toLocaleString()));
      g.appendChild(stat('Total supply', fmtTKM(latest.totalSupply, 2) + ' TKM'));
      g.appendChild(stat('Main King rewards', fmtTKM(latest.mainKingRewards, 2) + ' TKM'));
      g.appendChild(stat('Rotating King rewards', fmtTKM(latest.rotatingKingRewards, 2) + ' TKM'));
      g.appendChild(stat('Miner rewards', fmtTKM(latest.minerRewards, 2) + ' TKM'));
      g.appendChild(stat('Genesis supply', fmtTKM(latest.genesisSupply, 2) + ' TKM'));
      latestEl.body.appendChild(g);
    } catch (e) {
      latestEl.body.appendChild(el('div', { class: 'dim', text: e.message }));
    }

    histEl.body.innerHTML = '';
    const mkRow = (l, input) => el('div', { class: 'form-row' }, [el('label', { text: l }), input]);
    const atInput = el('input', { class: 'txt grow', placeholder: 'block number' });
    const atBtn = el('button', { class: 'btn secondary', text: 'Query' });
    const atOut = el('pre', { class: 'json-view' });
    atBtn.onclick = () => {
      const n = atInput.value.trim();
      if (!/^\d+$/.test(n)) { toast('Enter a block number.', 'err'); return; }
      runAction(atBtn, async () => {
        const r = await rpc('tkmsupply_atBlock', [Number(n)]);
        atOut.textContent = JSON.stringify(r, null, 2);
      });
    };
    histEl.body.appendChild(mkRow('Block', atInput));
    histEl.body.appendChild(atBtn);
    histEl.body.appendChild(atOut);

    syncEl.body.innerHTML = '';
    const syncTo = el('input', { class: 'txt grow', placeholder: 'extend the index to block' });
    const syncBtn = el('button', { class: 'btn', text: 'Sync', onclick: () => {
      const n = syncTo.value.trim();
      if (!/^\d+$/.test(n)) { toast('Enter a block number.', 'err'); return; }
      runAction(syncBtn, async () => {
        const r = await rpc('tkmsupply_sync', [Number(n)]);
        toast('Supply index synced to ' + (r ? (r.height ?? r.block ?? n) : n) + '.', 'ok');
      });
    } });
    syncEl.body.appendChild(el('div', { class: 'dim', text: 'Scans canonical blocks and persists cumulative totals for accounting.' }));
    syncEl.body.appendChild(mkRow('Sync to block', syncTo));
    syncEl.body.appendChild(syncBtn);
  }
})();