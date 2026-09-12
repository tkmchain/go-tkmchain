/* Kings — Rotating Kings (RK) governance. */
(function () {
  'use strict';
  const { rpc, register, el, card, stat, table, fmtHash, fmtHexNum, live, startTicker, runAction, toast, jsonView } = window.GUI;

  const section = {
    id: 'kings',
    label: 'Kings',
    icon: '♛',
    async render(view) {
      view.appendChild(card('Main King', '10% of the block reward'));
      view.appendChild(card('Rotating King', '40% · rotates every 100 blocks'));
      view.appendChild(card('Registered', 'rk_add / rk_list'));
      view.appendChild(card('Rotation history', 'rotatingking_getRotationHistory'));
      view.appendChild(card('Checkpoint', 'mainking checkpoint submission'));
      await live(view);
      await refreshKings(view);
      startTicker(section);
    },
    async refresh(view) {
      await refreshKings(view);
    },
  };
  register(section);

  async function refreshKings(view) {
    const cards = view.querySelectorAll('.card');
    const [mainEl, rotatingEl, registeredEl, historyEl, ckptEl] = cards;

    // Main King
    mainEl.body.innerHTML = '';
    try {
      const [mainAddress, info] = await Promise.all([
        rpc('mainking_address', []),
        rpc('mainking_getInfo', []).catch(() => null),
      ]);
      const g = el('div', { class: 'grid' });
      g.appendChild(stat('Main King', mainAddress || '—'));
      g.appendChild(stat('Main King (wrapped)', mainAddress ? fmtHash(mainAddress, 20) : '—'));
      mainEl.body.appendChild(g);
      if (info) mainEl.body.appendChild(jsonView(info));
    } catch (e) {
      mainEl.body.appendChild(el('div', { class: 'dim', text: e.message }));
    }

    // Rotating King
    rotatingEl.body.innerHTML = '';
    try {
      const ks = await rpc('rk_getKingStats', [null]);
      if (!ks) { rotatingEl.body.appendChild(empty('No stats.')); return; }
      const g = el('div', { class: 'grid' });
      g.appendChild(stat('Current King', ks.currentKing ? fmtHash(ks.currentKing, 16) : '—'));
      g.appendChild(stat('Next King', ks.nextKing ? fmtHash(ks.nextKing, 16) : '—'));
      g.appendChild(stat('Rotation interval', (ks.rotationInterval || 100) + ' blocks'));
      g.appendChild(stat('Current block', '#' + fmtHexNum(ks.currentBlock).toLocaleString()));
      g.appendChild(stat('Next rotation', '#' + fmtHexNum(ks.nextRotationHeight).toLocaleString()));
      g.appendChild(stat('Blocks until', String(fmtHexNum(ks.blocksUntilRotation))));
      rotatingEl.body.appendChild(g);
    } catch (e) {
      rotatingEl.body.appendChild(el('div', { class: 'dim', text: e.message }));
    }

    // Registered kings
    registeredEl.body.innerHTML = '';
    try {
      const list = await rpc('rk_list', []);
      const rows = (list || []).map((k) => [
        el('td', { text: fmtHash(k.address || k, 14) }),
        el('td', { text: k.registered ? 'yes' : 'no', class: 'r' }),
        el('td', { text: k.current ? 'current' : (k.next ? 'next' : '—') }),
        el('td', { class: 'dim', text: k.lockedAmount ? fmtTKM(k.lockedAmount, 2) + ' TKM' : '—' }),
      ]);
      if (!rows.length) registeredEl.body.appendChild(empty('No registered kings.'));
      else registeredEl.body.appendChild(table(['Address', 'Registered', 'Slot', 'Locked stake'], rows));
    } catch (e) {
      registeredEl.body.appendChild(el('div', { class: 'dim', text: e.message }));
    }

    // Rotation history
    historyEl.body.innerHTML = '';
    try {
      const hist = await rpc('rotatingking_getRotationHistory', [10]);
      const rows = (hist || []).map((h) => [
        el('td', { text: '#' + fmtHexNum(h.blockHeight).toLocaleString() }),
        el('td', { text: fmtHash(h.previousKing || '—', 12), class: 'dim' }),
        el('td', { text: fmtHash(h.newKing || '—', 12) }),
      ]);
      if (!rows.length) historyEl.body.appendChild(empty('No rotations yet.'));
      else historyEl.body.appendChild(table(['Height', 'Previous', 'New King'], rows));
    } catch (e) {
      historyEl.body.appendChild(el('div', { class: 'dim', text: e.message }));
    }

    // Checkpoint
    ckptEl.body.innerHTML = '';
    const nInput = el('input', { class: 'txt grow', placeholder: 'block number' });
    const hInput = el('input', { class: 'txt grow', placeholder: '0x block hash' });
    const subBtn = el('button', { class: 'btn gold', text: 'Add checkpoint', onclick: () => {
      const num = nInput.value.trim();
      const hash = hInput.value.trim();
      if (!/^\d+$/.test(num) || !/^0x[0-9a-fA-F]{64}$/.test(hash)) { toast('Enter a numeric block number and a 0x block hash.', 'err'); return; }
      runAction(subBtn, async () => {
        const ok = await rpc('rk_addCheckpoint', [Number(num), hash]);
        toast('Checkpoint accepted: ' + (ok === true ? 'yes' : (ok || JSON.stringify(ok))), 'ok');
      });
    } });
    ckptEl.body.appendChild(el('div', { class: 'dim', text: 'Submit a checkpoint (rk_addCheckpoint) for a signed block hash sent by the Main King node.' }));
    ckptEl.body.appendChild(el('div', { class: 'btn-row' }, [nInput, hInput, subBtn]));

    // reward split summary
    ckptEl.body.appendChild(el('div', { class: 'dim', style: 'margin-top:14px', text: 'Reward split: Main King 10% · Rotating King 40% · Miner 50%' }));
  }
})();