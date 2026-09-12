/* Dashboard — node + chain overview. */
(function () {
  'use strict';
  const { rpc, rpcBatch, register, el, card, stat, table, fmtTKM, fmtHash, fmtHexNum, fmtTS, live, startTicker, empty } = window.GUI;

  const section = {
    id: 'dashboard',
    label: 'Dashboard',
    icon: '◉',
    async render(view) {
      view.appendChild(liveCard());
      view.appendChild(chainCard());
      view.appendChild(syncCard());
      view.appendChild(peersCard());
      view.appendChild(kingsCard());
      await live(view);
      await refreshStatic(view);
      startTicker(section);
    },
    async refresh(view) {
      await refreshStatic(view);
    },
  };
  register(section);

  function liveCard() {
    return card('Node', 'web3 · eth · net');
  }

  function chainCard() { return card('Chain', 'latest block'); }
  function syncCard() { return card('Sync', 'download progress'); }
  function peersCard() { return card('Peers', 'network'); }
  function kingsCard() { return card('Governance', 'kings'); }

  async function refreshStatic(view) {
    const cards = view.querySelectorAll('.card');
    const [liveCardEl, chainEl, syncEl, peersEl, kingsEl] = cards;

    let syncing;
    try {
      const version = await rpc('web3_clientVersion', []);
      const [chainId, syncStatus, blockNumber] = await rpcBatch([
        { m: 'eth_chainId' }, { m: 'eth_syncing' }, { m: 'eth_blockNumber' },
      ]);
      syncing = syncStatus;
      const peers = await rpc('net_peerCount', []);
      const gasPrice = await rpc('eth_gasPrice', []);
      const mining = await rpc('eth_mining', []);
      const hashrate = await rpc('randomx_getHashrate', []);

      liveCardEl.body.innerHTML = '';
      const grid = el('div', { class: 'grid' });
      grid.appendChild(stat('Client', version || '—'));
      grid.appendChild(stat('Network', String(fmtHexNum(chainId))));
      grid.appendChild(stat('Head block', '#' + fmtHexNum(blockNumber).toLocaleString()));
      grid.appendChild(stat('Mining', mining ? '⛏ active' : 'idle', mining ? 'green' : 'dim'));
      grid.appendChild(stat('Hashrate', hashrate ? fmtHexNum(hashrate) + ' H/s' : '—'));
      grid.appendChild(stat('Peers', fmtHexNum(peers)));
      grid.appendChild(stat('Gas price', gasPrice ? fmtTKM(gasPrice, 4) + ' TKM' : '—'));
      liveCardEl.body.appendChild(grid);
    } catch (e) {
      liveCardEl.body.innerHTML = '';
      liveCardEl.body.appendChild(el('div', { class: 'empty', text: 'Offline: ' + e.message }));
    }

    // chain block details
    try {
      const blk = await rpc('eth_getBlockByNumber', ['latest', false]);
      chainEl.body.innerHTML = '';
      if (!blk) {
        chainEl.body.appendChild(empty('No block yet.'));
      } else {
        const g = el('div', { class: 'grid' });
        g.appendChild(stat('Number', '#' + fmtHexNum(blk.number).toLocaleString()));
        g.appendChild(stat('Hash', fmtHash(blk.hash, 16)));
        g.appendChild(stat('Parent', fmtHash(blk.parentHash, 12)));
        g.appendChild(stat('Miner', fmtHash(blk.miner, 10)));
        g.appendChild(stat('Time', fmtTS(blk.timestamp)));
        g.appendChild(stat('Transactions', fmtHexNum(blk.transactions ? blk.transactions.length : 0)));
        g.appendChild(stat('Gas used', fmtHexNum(blk.gasUsed)));
        g.appendChild(stat('Gas limit', fmtHexNum(blk.gasLimit)));
        chainEl.body.appendChild(g);
      }
    } catch (e) { chainEl.body.innerHTML = ''; chainEl.body.appendChild(empty(e.message)); }

    // sync progress
    syncEl.body.innerHTML = '';
    if (syncing && typeof syncing === 'object') {
      const cur = fmtHexNum(syncing.currentBlock) || 0;
      const high = fmtHexNum(syncing.highestBlock) || 0;
      const pct = high > 0 ? Math.min(100, Math.round((cur / high) * 100)) : 0;
      const g = el('div', { class: 'grid' });
      g.appendChild(stat('Mode', syncing.isSmartSync ? 'smart sync' : 'full/snap'));
      g.appendChild(stat('Progress', pct + '%'));
      g.appendChild(stat('At block', '#' + cur.toLocaleString()));
      g.appendChild(stat('Target', '#' + high.toLocaleString()));
      syncEl.body.appendChild(g);
      const prog = el('div', { class: 'progress' }, [el('div', { style: 'width:' + pct + '%' })]);
      syncEl.body.appendChild(prog);
    } else if (syncing === false) {
      syncEl.body.appendChild(el('div', { class: 'green', text: '✓ Fully synced (not syncing)' }));
    } else {
      syncEl.body.appendChild(el('div', { class: 'dim', text: 'Sync status unavailable.' }));
    }

    // peers
    peersEl.body.innerHTML = '';
    try {
      const pcount = await rpc('net_peerCount', []);
      peersEl.body.appendChild(el('div', { class: 'dim mono', text: (pcount ? fmtHexNum(pcount) : 0) + ' connected peer(s)' }));
      let rows = [];
      try {
        const adminPeers = await rpc('admin_peers', []);
        rows = (adminPeers || []).map((p) => [
          el('td', { text: p.enode ? fmtHash(p.enode, 12) : '—' }),
          el('td', { text: p.name ? fmtHash(p.name, 24) : '—' }),
          el('td', { text: p.network ? (p.network.inbound ? 'inbound' : 'outbound') : '—' }),
          el('td', { text: p.protocols ? Object.keys(p.protocols).join(', ') : '—' }),
        ]);
      } catch (e) {
        rows = [[el('td', { colspan: 4, text: 'admin_peers unavailable (' + e.message + ')' })]];
      }
      peersEl.body.appendChild(table(['Node', 'Name', 'Type', 'Protocols'], rows));
    } catch (e) {
      peersEl.body.appendChild(empty(e.message));
    }

    // kings quick view
    kingsEl.body.innerHTML = '';
    try {
      const mainKing = await rpc('mainking_address', []);
      const kingStats = await rpc('rk_getKingStats', [null]);
      const g = el('div', { class: 'grid' });
      g.appendChild(stat('Main King', mainKing ? fmtHash(mainKing, 12) : '—'));
      if (kingStats) {
        g.appendChild(stat('Current King', kingStats.currentKing ? fmtHash(kingStats.currentKing, 12) : '—'));
        g.appendChild(stat('Next King', kingStats.nextKing ? fmtHash(kingStats.nextKing, 12) : '—'));
        g.appendChild(stat('Registered', (kingStats.registeredKings || 0) + ' kings'));
        g.appendChild(stat('Rotation', 'every ' + fmtHexNum(kingStats.rotationInterval || 100) + ' blocks'));
        g.appendChild(stat('Next rotation', '#' + fmtHexNum(kingStats.nextRotationHeight).toLocaleString()));
      }
      kingsEl.body.appendChild(g);
    } catch (e) {
      kingsEl.body.appendChild(el('div', { class: 'dim', text: 'King info unavailable: ' + e.message }));
    }
  }
})();