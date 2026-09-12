/* Console — generic JSON-RPC / web3 API explorer for every namespace. */
(function () {
  'use strict';
  const { rpc, register, el, card, live, toast, jsonView } = window.GUI;

  const section = {
    id: 'console',
    label: 'API / web3',
    icon: '⌘',
    async render(view) {
      view.appendChild(card('Namespaces', 'rpc_modules'));
      view.appendChild(card('Call', 'method + JSON params'));
      view.appendChild(card('History', 'last calls'));
      const modulesCard = view.querySelectorAll('.card')[0];
      const callCard = view.querySelectorAll('.card')[1];
      const histCard = view.querySelectorAll('.card')[2];
      await renderConsole(view, modulesCard, callCard, histCard);
      await live(view);
    },
  };
  register(section);

  async function renderConsole(view, modulesCard, callCard, histCard) {
    modulesCard.body.innerHTML = '';
    const mods = await rpc('rpc_modules', []).catch(() => ({}));
    const names = Object.keys(mods || {});
    const chips = el('div', { class: 'btn-row' });
    for (const ns of names.sort()) {
      const chip = el('button', { class: 'btn secondary mono', text: ns, onclick: () => {
        methodInput.value = ns + '.';
        methodInput.focus();
      } });
      chip.style.fontSize = '11.5px';
      chip.style.padding = '5px 9px';
      chips.appendChild(chip);
    }
    modulesCard.body.appendChild(el('div', { class: 'dim', text: names.length ? names.length + ' namespaces exposed by this node:' : 'rpc_modules unavailable.' }));
    modulesCard.body.appendChild(chips);

    const mkRow = (l, input) => el('div', { class: 'form-row' }, [el('label', { text: l }), input]);
    const methodInput = el('input', { class: 'txt', placeholder: 'e.g. eth_getBlockByNumber' });
    const paramsInput = el('textarea', { class: 'txt', placeholder: '["latest", false]  — JSON array, optional' });
    const execBtn = el('button', { class: 'btn green', text: 'Run' });
    const out = el('pre', { class: 'json-view' });

    const run = async () => {
      const method = methodInput.value.trim();
      if (!method) { toast('Enter a method, e.g. eth_getBlockByNumber.', 'err'); return; }
      let params = [];
      const raw = paramsInput.value.trim();
      if (raw) {
        try { params = JSON.parse(raw); } catch (e) { toast('Params must be a JSON array.', 'err'); return; }
        if (!Array.isArray(params)) { toast('Params must be a JSON array.', 'err'); return; }
      }
      setBusy(execBtn, true);
      try {
        const result = await rpc(method, params);
        out.textContent = JSON.stringify(result, null, 2);
        addHistory(method, params, result);
      } catch (e) {
        out.textContent = 'ERROR: ' + e.message;
        addHistory(method, params, e.message);
      } finally {
        setBusy(execBtn, false);
      }
    };
    execBtn.onclick = run;
    methodInput.addEventListener('keydown', (e) => { if (e.key === 'Enter') run(); });

    callCard.body.appendChild(el('div', { class: 'dim', text: 'Call any JSON-RPC method exposed by the node in-process client (all namespaces, including phone, mail, kings, mining, shielded privacy, and the web3 extension data.).' }));
    callCard.body.appendChild(mkRow('Method', methodInput));
    callCard.body.appendChild(mkRow('Params (JSON array)', paramsInput));
    callCard.body.appendChild(execBtn);
    callCard.body.appendChild(out);

    histCard.body.appendChild(el('div', { id: 'hist-list' }));

    function addHistory(method, params, result) {
      const hist = document.getElementById('hist-list');
      const entry = el('div', { class: 'dim mono', style: 'padding:6px 0;border-bottom:1px solid var(--border)', text: '› ' + method + ' ' + JSON.stringify(params) });
      entry.title = typeof result === 'string' ? result : JSON.stringify(result).slice(0, 400);
      if (hist.firstChild) hist.insertBefore(entry, hist.firstChild);
      else hist.appendChild(entry);
      while (hist.childNodes.length > 25) hist.removeChild(hist.lastChild);
    }
  }

  function setBusy(btn, busy) {
    if (busy) { btn.disabled = true; btn.textContent = 'running…'; }
    else { btn.disabled = false; btn.textContent = 'Run'; }
  }
})();