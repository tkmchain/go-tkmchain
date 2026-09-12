/* Governance — tkmgov disclosure ledger. */
(function () {
  'use strict';
  const { rpc, register, el, card, stat, table, fmtHash, fmtHexNum, fmtTS, live, runAction, toast, jsonView } = window.GUI;

  const section = {
    id: 'governance',
    label: 'Governance',
    icon: '⚖',
    async render(view) {
      view.appendChild(card('Latest disclosure', 'tkmgov_latestDisclosure'));
      view.appendChild(card('Ledger', 'tkmgov_listDisclosures'));
      view.appendChild(card('Publish', 'signed by the Main King keystore account'));
      view.appendChild(card('Verify', 'tkmgov_verifyDisclosure'));
      await live(view);
      await refreshGov(view);
    },
    async refresh(view) {
      await refreshGov(view);
    },
  };
  register(section);

  async function refreshGov(view) {
    const cards = view.querySelectorAll('.card');
    const [latestEl, ledgerEl, publishEl, verifyEl] = cards;

    latestEl.body.innerHTML = '';
    try {
      const d = await rpc('tkmgov_latestDisclosure', ['']);
      if (!d) { latestEl.body.appendChild(empty('No disclosures yet.')); return; }
      if (d.error) { latestEl.body.appendChild(el('div', { class: 'dim', text: d.error.message || JSON.stringify(d.error) })); return; }
      const g = el('div', { class: 'grid' });
      g.appendChild(stat('ID', String(fmtHexNum(d.id))));
      g.appendChild(stat('Kind', d.kind || '—'));
      g.appendChild(stat('Title', d.title || '—'));
      g.appendChild(stat('Content hash', d.contentHash ? fmtHash(d.contentHash, 14) : '—'));
      g.appendChild(stat('Disclosure hash', d.disclosureHash ? fmtHash(d.disclosureHash, 14) : '—'));
      g.appendChild(stat('Anchor tx', d.anchorTx ? fmtHash(d.anchorTx, 12) : '—'));
      g.appendChild(stat('Date', fmtTS(d.createdAt)));
      latestEl.body.appendChild(g);
      latestEl.body.appendChild(jsonView(d));
    } catch (e) {
      latestEl.body.appendChild(el('div', { class: 'dim', text: e.message }));
    }

    ledgerEl.body.innerHTML = '';
    const kindInput = el('input', { class: 'txt grow', placeholder: 'kind filter (empty = all, e.g. protocol)' });
    const loadBtn = el('button', { class: 'btn secondary', text: 'Load', onclick: () => {
      runAction(loadBtn, async () => {
        const kind = kindInput.value.trim();
        const list = (await rpc('tkmgov_listDisclosures', [kind, '0x0', '0x64'])) || [];
        ledgerEl.out.innerHTML = '';
        const rows = list.map((d) => [
          el('td', { text: String(fmtHexNum(d.id)) }),
          el('td', { text: d.kind || '—' }),
          el('td', { text: d.title ? fmtHash(d.title, 18) : '—' }),
          el('td', { text: d.contentHash ? fmtHash(d.contentHash, 10) : '—' }),
          el('td', { class: 'dim', text: fmtTS(d.createdAt) }),
        ]);
        if (!rows.length) ledgerEl.out.appendChild(empty('No disclosures for filter "' + (kind || 'all') + '".'));
        else ledgerEl.out.appendChild(table(['ID', 'Kind', 'Title', 'Content hash', 'Date'], rows));
      });
    } });
    ledgerEl.body.appendChild(el('div', { class: 'btn-row' }, [kindInput, loadBtn]));
    ledgerEl.out = el('div');
    ledgerEl.body.appendChild(ledgerEl.out);

    publishEl.body.innerHTML = '';
    const mkRow = (l, input) => el('div', { class: 'form-row' }, [el('label', { text: l }), input]);
    const kInput = el('input', { class: 'txt', placeholder: 'kind (e.g. protocol)' });
    const tInput = el('input', { class: 'txt grow', placeholder: 'title' });
    const cInput = el('input', { class: 'txt grow', placeholder: '0x… content hash' });
    const uriInput = el('input', { class: 'txt grow', placeholder: 'uri (optional)' });
    const prevInput = el('input', { class: 'txt grow', placeholder: '0x… previous disclosure hash (optional)' });
    const acctInput = el('input', { class: 'txt grow', placeholder: '0x Main King account' });
    const passInput = el('input', { class: 'txt grow', type: 'password', placeholder: 'account passphrase' });
    const pubBtn = el('button', { class: 'btn green', text: 'Hash · sign · publish' });

    pubBtn.onclick = () => {
      const kind = kInput.value.trim();
      const title = tInput.value.trim();
      const content = cInput.value.trim();
      const uri = uriInput.value.trim();
      const previous = prevInput.value.trim() || '0x' + '0'.repeat(64);
      const acct = acctInput.value.trim();
      const pass = passInput.value;
      if (!kind || !title) { toast('Kind and title are required.', 'err'); return; }
      if (!/^0x[0-9a-fA-F]{64}$/.test(content)) { toast('Enter a valid content hash.', 'err'); return; }
      if (previous !== '0x' + '0'.repeat(64) && !/^0x[0-9a-fA-F]{64}$/.test(previous)) { toast('Previous hash must be 0x… (64 hex chars) or empty.', 'err'); return; }
      if (!/^0x[0-9a-fA-F]{40}$/.test(acct) || !pass) { toast('Enter the Main King account and its passphrase.', 'err'); return; }
      runAction(pubBtn, async () => {
        const version = '0x1';
        const ts = '0x' + Math.floor(Date.now() / 1000).toString(16);
        const digest = await rpc('tkmgov_disclosureHash', [kind, title, version, content, uri, previous, ts]);
        const sig = await rpc('tkm_signHashWithPassphrase', [acct, digest, pass]);
        const rec = await rpc('tkmgov_publishDisclosure', [kind, title, version, content, uri, previous, ts, '0x' + '0'.repeat(64), sig]);
        toast('Published disclosure #' + String(rec && rec.id) + ' (' + fmtHash(rec && rec.disclosureHash, 12) + ')', 'ok');
      });
    };
    publishEl.body.appendChild(el('div', { class: 'dim', text: 'Computes the canonical disclosure hash (tkmgov_disclosureHash), signs it with the Main King keystore account, then publishes. Anchor tx may be omitted.' }));
    publishEl.body.appendChild(mkRow('Kind', kInput));
    publishEl.body.appendChild(mkRow('Title', tInput));
    publishEl.body.appendChild(mkRow('Content hash', cInput));
    publishEl.body.appendChild(mkRow('URI', uriInput));
    publishEl.body.appendChild(mkRow('Previous hash', prevInput));
    publishEl.body.appendChild(mkRow('Main King account', acctInput));
    publishEl.body.appendChild(mkRow('Passphrase', passInput));
    publishEl.body.appendChild(pubBtn);

    verifyEl.body.innerHTML = '';
    const vInput = el('input', { class: 'txt grow', placeholder: 'disclosure ID' });
    const vBtn = el('button', { class: 'btn secondary', text: 'Verify', onclick: () => {
      const id = vInput.value.trim();
      if (!/^\d+$/.test(id)) { toast('Enter a numeric disclosure ID.', 'err'); return; }
      runAction(vBtn, async () => {
        const r = await rpc('tkmgov_verifyDisclosure', [Number(id)]);
        toast('Verification: ' + (r === true ? 'valid ✓' : JSON.stringify(r)), r === true ? 'ok' : '');
        verifyEl.out.appendChild(jsonView(r));
        await new Promise((resolve) => setTimeout(resolve, 4000));
        verifyEl.out.innerHTML = '';
      });
    } });
    verifyEl.body.appendChild(el('div', { class: 'btn-row' }, [vInput, vBtn]));
    verifyEl.out = el('div');
    verifyEl.body.appendChild(verifyEl.out);
  }
})();