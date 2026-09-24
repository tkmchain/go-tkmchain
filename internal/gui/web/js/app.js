/* gtkm GUI — core application. Vanilla JS, no build step. */
(function () {
  'use strict';

  const tokenMeta = document.querySelector('meta[name="gui-token"]');
  const TOKEN = tokenMeta ? tokenMeta.getAttribute('content') : '';

  const GUI = {
    sections: [],
    current: null,
    tickers: {},
    network: null,
    version: null,
  };

  /* ---------- JSON-RPC client ---------- */

  async function rpc(method, params) {
    const body = { jsonrpc: '2.0', id: 1, method, params: params || [] };
    const res = await fetch('/rpc', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-GUI-Token': TOKEN },
      body: JSON.stringify(body),
    });
    if (res.status === 403) throw new Error('RPC forbidden (bad token)');
    if (res.status === 204) return null;
    const text = await res.text();
    if (!text) return null;
    const data = JSON.parse(text);
    if (data.error) {
      const e = new Error(data.error.message || ('RPC error ' + data.error.code));
      e.code = data.error.code;
      throw e;
    }
    return data.result;
  }

  async function rpcBatch(calls) {
    const body = calls.map((c, i) => ({ jsonrpc: '2.0', id: i + 1, method: c.m, params: c.p || [] }));
    const res = await fetch('/rpc', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-GUI-Token': TOKEN },
      body: JSON.stringify(body),
    });
    if (res.status !== 200) throw new Error('batch RPC failed: ' + res.status);
    const data = await res.json();
    data.sort((a, b) => a.id - b.id);
    return data.map((d) => {
      if (d.error) {
        const e = new Error(d.error.message || 'RPC error ' + d.error.code);
        e.code = d.error.code;
        throw e;
      }
      return d.result;
    });
  }

  async function bootstrapDownload() {
    const res = await fetch('/bootstrap', {
      method: 'POST',
      headers: { 'X-GUI-Token': TOKEN },
    });
    const text = await res.text();
    let data = {};
    try { data = text ? JSON.parse(text) : {}; } catch (_) {}
    if (!res.ok) throw new Error(data.error || ('bootstrap download failed (' + res.status + ')'));
    return data;
  }

  /* ---------- helpers ---------- */

  function el(tag, attrs, children) {
    const node = document.createElement(tag);
    if (attrs) {
      for (const [k, v] of Object.entries(attrs)) {
        if (k === 'class') node.className = v;
        else if (k === 'text') node.textContent = v;
        else if (k === 'html') node.innerHTML = v;
        else if (k.startsWith('on') && typeof v === 'function') node.addEventListener(k.slice(2), v);
        else if (k === 'value') node.value = v;
        else node.setAttribute(k, v);
      }
    }
    if (children != null) {
      const list = Array.isArray(children) ? children : [children];
      for (const c of list) {
        if (c == null || c === false) continue;
        node.appendChild(typeof c === 'string' ? document.createTextNode(c) : c);
      }
    }
    return node;
  }

  function card(title, hint) {
    const h = el('h2', {}, [title, hint ? el('span', { class: 'hint dim', text: hint }) : null]);
    const c = el('div', { class: 'card' }, [h]);
    c.body = el('div');
    c.appendChild(c.body);
    return c;
  }

  function stat(k, v, cls) {
    return el('div', { class: 'stat' }, [el('div', { class: 'k', text: k }), el('div', { class: 'v ' + (cls || ''), text: v == null ? '—' : String(v) })]);
  }

  function empty(msg) { return el('div', { class: 'empty', text: msg || 'No data.' }); }

  function table(headers, rows) {
    const thead = el('thead', {}, [el('tr', {}, headers.map((h) => el('th', { text: h })))]);
    const tbody = el('tbody', {}, rows.map((r) => el('tr', {}, r)));
    const tbl = el('table', { class: 'tbl' }, [thead, tbody]);
    return el('div', { class: 'tbl-wrap' }, [tbl]);
  }

  function jsonView(obj) {
    return el('pre', { class: 'json-view', text: typeof obj === 'string' ? obj : safeJSON(obj) });
  }

  function safeJSON(obj) {
    try { return JSON.stringify(obj, null, 2); } catch (e) { return String(obj); }
  }

  function toast(msg, type) {
    const t = el('div', { class: 'toast ' + (type || ''), text: msg });
    document.getElementById('toasts').appendChild(t);
    setTimeout(() => t.remove(), 5000);
  }

  /* TKM wei formatting: 18 decimals. Accepts hex strings (JSON hexutil.Big),
   * decimal strings, or numbers. */
  function fmtTKM(wei, digits) {
    if (wei == null) return '—';
    let s = String(wei);
    if (/^0x/i.test(s)) s = BigInt(s).toString(10);
    const neg = s.startsWith('-');
    const clean = s.replace(/^-/, '');
    let num = clean.padStart(19, '0');
    const whole = num.slice(0, -18).replace(/^0+(?=\d)/, '') || '0';
    const frac = num.slice(-18);
    const d = digits == null ? 6 : digits;
    return (neg ? '-' : '') + whole + '.' + frac.slice(0, d);
  }

  function fmtHash(h, n) {
    if (!h) return '—';
    const s = String(h);
    if (s.length < 2 * (n || 8)) return s;
    const half = n || 8;
    return s.slice(0, half) + '…' + s.slice(-half);
  }

  function fmtHexNum(v) {
    if (v == null) return '—';
    if (typeof v === 'number') return String(v);
    const s = String(v);
    if (s.startsWith('0x')) return parseInt(s, 16).toLocaleString();
    return s;
  }

  function fmtTS(hexOrNum) {
    if (hexOrNum == null) return '—';
    const n = typeof hexOrNum === 'string' && hexOrNum.startsWith('0x') ? parseInt(hexOrNum, 16) : Number(hexOrNum);
    if (!n) return '—';
    return new Date(n * 1000).toLocaleString();
  }

  function setBusy(btn, busy, label) {
    if (!btn) return;
    if (busy) {
      btn.dataset.label = btn.textContent;
      btn.disabled = true;
      btn.innerHTML = '<span class="spin"></span> ' + (label || 'working…');
    } else {
      btn.disabled = false;
      btn.textContent = btn.dataset.label || btn.textContent;
    }
  }

  async function runAction(btn, action, okMsg) {
    setBusy(btn, true);
    try {
      const result = await action();
      if (okMsg) toast(okMsg, 'ok');
      return result;
    } catch (e) {
      toast('Error: ' + e.message, 'err');
      throw e;
    } finally {
      setBusy(btn, false);
    }
  }

  /* ---------- navigation + polling ---------- */

  function register(section) {
    GUI.sections.push(section);
  }

  function renderNav() {
    const nav = document.getElementById('nav');
    nav.innerHTML = '';
    for (const s of [...GUI.sections].sort((a, b) => (a.id === 'wallet' ? -1 : b.id === 'wallet' ? 1 : 0))) {
      const item = el('button', {
        type: 'button',
        'aria-current': s === GUI.current ? 'page' : 'false',
        class: 'nav-item' + (s === GUI.current ? ' active' : ''),
        onclick: () => activate(s),
      }, [el('span', { class: 'ico', text: s.icon || '•' }), el('span', {}, [s.label])]);
      nav.appendChild(item);
    }
  }

  async function activate(section) {
    pauseTickers();
    if (GUI.current?.onHidden) GUI.current.onHidden();
    GUI.current = section;
    renderNav();
    document.getElementById('page-title').textContent = section.label;
    const view = document.getElementById('view');
    view.innerHTML = '';
    view.dataset.section = section.id;
    delete view.dataset.live;
    try {
      await section.render(view);
      if (section.onVisible) section.onVisible();
    } catch (e) {
      view.appendChild(el('div', { class: 'empty', text: 'Failed to load: ' + e.message }));
    }
  }

  function pauseTickers() {
    for (const id of Object.keys(GUI.tickers)) {
      const t = GUI.tickers[id];
      if (t && t.timer) { clearInterval(t.timer); t.timer = null; }
    }
  }

  function startTicker(section) {
    pauseTickers();
    if (!section.refresh) return;
    const run = async () => {
      if (document.hidden || GUI.current !== section) return;
      const view = document.getElementById('view');
      if (!view.dataset.live) return;
      try { await section.refresh(view); } catch (e) { /* keep last content */ }
    };
    const timer = setInterval(run, 2500);
    GUI.tickers[section.id] = { timer };
  }

  async function live(view) {
    view.dataset.live = '1';
  }

  async function stopLive(view) {
    delete view.dataset.live;
  }

  function setConn(ok, label) {
    const dot = document.getElementById('conn-dot');
    const lbl = document.getElementById('conn-label');
    dot.classList.toggle('on', ok);
    lbl.textContent = label || (ok ? 'connected' : 'offline');
  }

  /* ---------- boot ---------- */

  async function boot() {
    setConn(false, 'connecting…');
    const health = await fetch('/healthz').then((res) => res.ok).catch(() => false);
    // derive version + network for the topbar
    try {
      GUI.version = await rpc('web3_clientVersion', []);
    } catch (e) { GUI.version = null; }
    try {
      const chainId = await rpc('eth_chainId', []);
      const peers = await rpc('net_peerCount', []);
      GUI.network = { chainId: fmtHexNum(chainId), peers: fmtHexNum(peers) };
    } catch (e) { /* not ready yet */ }
    setConn(health, health ? 'connected' : 'offline');
    renderNav();
    await activate(GUI.sections.find(s => s.id === 'wallet') || GUI.sections[0]);

    // keep the connection indicator + topbar meta fresh
    setInterval(async () => {
      try {
        const [chainId, block, syncing, peers] = await rpcBatch([
          { m: 'eth_chainId' }, { m: 'eth_blockNumber' },
          { m: 'eth_syncing' }, { m: 'net_peerCount' },
        ]);
        GUI.currentBlock = fmtHexNum(block);
        document.getElementById('topbar-meta').textContent =
          'chain ' + fmtHexNum(chainId) + ' · block #' + GUI.currentBlock.toLocaleString() +
          (syncing ? ' · syncing' : ' · synced') +
          ' · ' + fmtHexNum(peers) + ' peers';
        setConn(true, 'connected');
      } catch (e) {
        setConn(false, 'reconnecting…');
      }
    }, 3000);
  }

  GUI.rpcToken = () => TOKEN;
  GUI.engine = () => import("/engine/wallet.js");
  GUI.rpc = rpc;
  GUI.rpcBatch = rpcBatch;
  GUI.bootstrap = bootstrapDownload;
  GUI.register = register;
  GUI.activate = activate;
  GUI.startTicker = startTicker;
  GUI.pauseTickers = pauseTickers;
  GUI.live = live;
  GUI.stopLive = stopLive;
  GUI.el = el;
  GUI.card = card;
  GUI.stat = stat;
  GUI.empty = empty;
  GUI.table = table;
  GUI.jsonView = jsonView;
  GUI.toast = toast;
  GUI.fmtTKM = fmtTKM;
  GUI.fmtHash = fmtHash;
  GUI.fmtHexNum = fmtHexNum;
  GUI.fmtTS = fmtTS;
  GUI.setBusy = setBusy;
  GUI.runAction = runAction;

  window.GUI = GUI;
  document.addEventListener('DOMContentLoaded', boot);
  // Installable PWA (browser/phone); harmless inside the desktop webview.
  if ('serviceWorker' in navigator && window.location.protocol.startsWith('http')) {
    window.addEventListener('load', () => {
      navigator.serviceWorker.register('/sw.js').catch(() => {});
    });
  }
})();