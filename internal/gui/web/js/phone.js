/* Phone — TKM Phone: buckets, numbers, encrypted messages. */
(function () {
  'use strict';
  const { rpc, register, el, card, stat, table, fmtHash, fmtHexNum, fmtTS, live, runAction, toast, jsonView, empty, fmtTKM } = window.GUI;

  const section = {
    id: 'phone',
    label: 'Phone',
    icon: '✆',
    async render(view) {
      view.appendChild(card('Status', 'tkmphone_status'));
      view.appendChild(card('Buckets', 'MainKing-signed 5-number buckets'));
      view.appendChild(card('Operators', 'operator sales'));
      view.appendChild(card('Registered numbers', 'SIM / device keys'));
      view.appendChild(card('Send encrypted message', 'number-to-number'));
      const registration = card('Register your device', 'Activate a number you already own');
      view.appendChild(registration);
      renderRegistration(registration);
      const migration = card('Move a number to PQ', 'Existing numbers keep their identity');
      view.appendChild(migration); renderMigration(migration);
      await live(view);
      await refreshPhone(view);
    },
    async refresh(view) {
      await refreshPhone(view);
    },
  };
  register(section);

  async function refreshPhone(view) {
    const cards = view.querySelectorAll('.card');
    const [statusEl, bucketsEl, operatorsEl, numbersEl, sendEl] = cards;

    statusEl.body.innerHTML = '';
    try {
      const s = await rpc('tkmphone_status', []);
      const g = el('div', { class: 'grid' });
      g.appendChild(stat('Phone fork active', s.active ? 'yes' : 'no', s.active ? 'green' : 'warn'));
      g.appendChild(stat('Head', s.headNumber != null ? '#' + fmtHexNum(s.headNumber).toLocaleString() : '—'));

      statusEl.body.appendChild(g);
    } catch (e) {
      statusEl.body.appendChild(el('div', { class: 'dim', text: e.message }));
    }

    bucketsEl.body.innerHTML = '';
    try {
      const buckets = (await rpc('tkmphone_buckets', [])) || [];
      const rows = buckets.map((b) => [
        el('td', { text: fmtHash(b.bucketHash || b.hash || '—', 12) }),
        el('td', { text: fmtHash(b.operator || '—', 10) }),
        el('td', { text: b.creationTx ? fmtHash(b.creationTx, 10) : '—' }),
        el('td', { text: b.paymentTx ? fmtHash(b.paymentTx, 10) : '—' }),
      ]);
      if (!rows.length) bucketsEl.body.appendChild(empty('No buckets yet.'));
      else bucketsEl.body.appendChild(table(['Bucket', 'Operator', 'Creation tx', 'Payment tx'], rows));
    } catch (e) {
      bucketsEl.body.appendChild(el('div', { class: 'dim', text: e.message }));
    }

    operatorsEl.body.innerHTML = '';
    try {
      const ops = (await rpc('tkmphone_listOperators', [])) || [];
      const rows = ops.map((o) => [
        el('td', { text: fmtHash(o.operator || o.address || '—', 12) }),
        el('td', { text: o.bucketId != null ? fmtHexNum(o.bucketId) : '—' }),
        el('td', { text: o.active ? 'active' : 'inactive', class: 'r' }),
      ]);
      if (!rows.length) operatorsEl.body.appendChild(empty('No operators.'));
      else operatorsEl.body.appendChild(table(['Operator', 'Bucket ID', 'Status'], rows));
    } catch (e) {
      operatorsEl.body.appendChild(el('div', { class: 'dim', text: e.message }));
    }

    numbersEl.body.innerHTML = '';
    try {
      const nums = (await rpc('tkmphone_registeredNumbers', [])) || [];
      const rows = nums.map((n) => [
        el('td', { text: n.number.number || '—' }),
        el('td', { text: fmtHash(n.number.owner || '—', 10) }),
        el('td', { text: (n.devices || []).map(d => d.device + (d.active ? ' (active)' : ' (inactive)')).join(', ') || 'Not registered' }),
      ]);
      if (!rows.length) numbersEl.body.appendChild(empty('No registered numbers yet.'));
      else numbersEl.body.appendChild(table(['Number', 'Owner', 'Device key'], rows));
    } catch (e) {
      numbersEl.body.appendChild(el('div', { class: 'dim', text: e.message }));
    }

    renderSend(sendEl);
  }

  async function renderSend(container) {
    container.body.innerHTML = '';
    const mkRow = (label, input) => el('div', { class: 'form-row' }, [el('label', { text: label }), input]);

    const fromInput = el('input', { class: 'txt', placeholder: 'your owned number' });
    const toInput = el('input', { class: 'txt', placeholder: 'recipient number' });
    const msgInput = el('textarea', { class: 'txt', placeholder: 'plaintext message' });
    const ownerInput = el('input', { class: 'txt', placeholder: '0x owner account with registered device key' });
    const passInput = el('input', { class: 'txt', placeholder: 'owner account passphrase', type: 'password' });
    const btn = el('button', { class: 'btn green', text: 'Encrypt + sign + send' });

    btn.onclick = () => {
      const from = fromInput.value.trim();
      const to = toInput.value.trim();
      const text = msgInput.value;
      const owner = ownerInput.value.trim();
      const pass = passInput.value;
      if (!from || !to || !text) { toast('Fill number from, number to, and message.', 'err'); return; }
      if (!/^0x[0-9a-fA-F]{40}$/.test(owner) || !pass) { toast('Owner address and passphrase are required to sign.', 'err'); return; }
      runAction(btn, async () => {
        const nonce = '0x' + randomBytesHex(16);
        const plain = toHex(text);
        const cipher = await rpc('tkmphone_encryptPayload', [from, to, nonce, plain]);
        const sigHash = await rpc('tkmphone_sendMessageSigningHash', [from, to, nonce, cipher.ciphertext]);
        const sig = await signPhone(owner, pass, sigHash);
        const msg = await rpc('tkmphone_sendEncryptedMessage', [from, to, cipher.ciphertext, nonce, sig]);
        toast('Message stored with id ' + (msg.id != null ? fmtHexNum(msg.id) : 'unknown'), 'ok');
      }).catch(() => {}).finally(() => { passInput.value = ''; });
    };

    container.body.appendChild(el('div', { class: 'dim', text: 'Both numbers must have active device keys; the message is encrypted on-chain and only decryptable by the recipient.' }));
    container.body.appendChild(mkRow('From number', fromInput));
    container.body.appendChild(mkRow('To number', toInput));
    container.body.appendChild(mkRow('Message', msgInput));
    container.body.appendChild(mkRow('Owner account (node keystore)', ownerInput));
    container.body.appendChild(mkRow('Passphrase', passInput));
    container.body.appendChild(btn);
  }

  function renderRegistration(container) {
    const field = (label, placeholder, type = 'text') => {
      const input = el('input', {class: 'txt', placeholder, type, 'aria-label': label, autocomplete: type === 'password' ? 'current-password' : 'off'});
      container.body.appendChild(el('div', {class: 'form-row'}, [el('label', {}, [label, input])]));
      return input;
    };
    container.body.appendChild(el('p', {class: 'dim', text: 'Register a device with your PQ wallet. Leave the public key blank to use this wallet’s ML-DSA-87 identity, recoverable with its 24 words. Existing legacy owners can migrate their number below.'}));
    const number = field('Phone number', 'Your purchased number');
    const device = field('Device name', 'My Android phone');
    const publicKey = field('Device public key (optional)', 'Leave blank to use the owner’s PQ public key');
    const owner = field('Owner account', '0x… account in this node');
    const password = field('Wallet password', 'Wallet password', 'password');
    const out = el('div', {'aria-live': 'polite'});
    const lookup = el('button', {class: 'btn secondary', text: 'Check registration'});
    const submit = el('button', {class: 'btn', text: 'Sign and register device'});
    lookup.onclick = () => runAction(lookup, async () => {
      if (!number.value.trim()) throw Error('Enter your phone number.');
      const record = await rpc('tkmphone_registeredNumber', [number.value.trim()]);
      out.innerHTML = ''; out.appendChild(jsonView(record));
      if (record.number?.owner) owner.value = record.number.owner;
    }).catch(() => {});
    submit.onclick = () => runAction(submit, async () => {
      const n = number.value.trim(), d = device.value.trim(), account = owner.value.trim();
      let key = publicKey.value.trim();
      try {
        if (!n || !d || !/^0x[a-fA-F0-9]{40}$/.test(account) || !password.value) throw Error('Enter a number, device name, hex public key, owner account and password.');
        const algorithms = await rpc('tkm_accountAlgorithms', []);
        const algorithm = Object.entries(algorithms || {}).find(([address]) => address.toLowerCase() === account.toLowerCase())?.[1];
        if (algorithm !== 'ML-DSA-87') throw Error('Select a PQ owner account. Use the migration form for a legacy-owned number.');
        const engine = await GUI.engine();
        const keyfile = engine.keyfileFromHex(await rpc('tkm_exportPQAccount', [account, password.value, password.value]));
        if (!key) key = '0x' + keyfile.publicKey.replace(/^0x/, '');
        if (!/^0x[0-9a-fA-F]{5184}$/.test(key)) throw Error('A device key must be a complete ML-DSA-87 public key.');
        const current = await rpc('tkmphone_registeredNumber', [n]);
        if (current.number.owner.toLowerCase() !== account.toLowerCase()) throw Error('This account does not own the number.');
        if ((current.devices || []).some(x => x.device === d && x.active && x.publicKey.toLowerCase() === key.toLowerCase())) throw Error('This device is already registered.');
        const hash = await rpc('tkmphone_deviceKeySigningHash', [n, d, key]);
        const supported = await rpc('tkmphone_signatureAlgorithms', []);
        if (!supported.includes('ML-DSA-87')) throw Error('Update the Phone node to enable PQ signatures.');
        const signature = await engine.signPhoneDigest(keyfile, password.value, hash);
        const result = await rpc('tkmphone_registerDeviceKey', [n, d, key, signature]);
        out.innerHTML = ''; out.appendChild(jsonView(result));
        toast('Device registered successfully.', 'ok');
      } finally { password.value = ''; }
    }).catch(() => {});
    container.body.appendChild(lookup); container.body.appendChild(submit); container.body.appendChild(out);
  }

  async function signPhone(owner, password, digest, allowLegacy = false) {
    const algorithms = await rpc('tkm_accountAlgorithms', []);
    const algorithm = Object.entries(algorithms || {}).find(([address]) => address.toLowerCase() === owner.toLowerCase())?.[1];
    if (algorithm === 'ML-DSA-87') {
      const supported = await rpc('tkmphone_signatureAlgorithms', []);
      if (!supported.includes('ML-DSA-87')) throw Error('Update the Phone node to enable PQ signatures.');
      const engine = await GUI.engine();
      const keyfile = engine.keyfileFromHex(await rpc('tkm_exportPQAccount', [owner, password, password]));
      return engine.signPhoneDigest(keyfile, password, digest);
    }
    if (allowLegacy && algorithm === 'ECDSA-secp256k1') return rpc('tkm_signHashWithPassphrase', [owner, digest, password]);
    throw Error('Use a PQ owner account. Legacy owners must migrate their number first.');
  }

  function renderMigration(container) {
    const field = (label, type = 'text') => {
      const input = el('input', {class:'txt',type,'aria-label':label});
      container.body.appendChild(el('div',{class:'form-row'},[el('label',{},[label,input])])); return input;
    };
    const number=field('Number to migrate'), owner=field('Current owner account'), target=field('New PQ owner account'), password=field('Current owner password','password');
    container.body.appendChild(el('p',{class:'dim',text:'Ownership will move to the new PQ account. Existing device keys and recovery authority will be removed; register your device again afterward.'}));
    const submit=el('button',{class:'btn',text:'Sign and move number to PQ'}), out=el('div',{'aria-live':'polite'});
    submit.onclick=()=>runAction(submit,async()=>{
      try {
        const n=number.value.trim(),from=owner.value.trim(),to=target.value.trim();
        if(!n || !password.value || !/^0x[0-9a-fA-F]{40}$/.test(from) || !/^0x[0-9a-fA-F]{40}$/.test(to))throw Error('Enter the number, both owner addresses and current owner password.');
        const algorithms=await rpc('tkm_accountAlgorithms',[]);
        if(!Object.entries(algorithms).some(([a,k])=>a.toLowerCase()===to.toLowerCase()&&k==='ML-DSA-87'))throw Error('Import or create the destination PQ wallet on this node first.');
        const record=await rpc('tkmphone_number',[n]);
        if(record.owner.toLowerCase()!==from.toLowerCase())throw Error('Current owner does not match the number.');
        const supported=await rpc('tkmphone_signatureAlgorithms',[]);if(!supported.includes('ML-DSA-87'))throw Error('Update the Phone node before migrating.');
        const digest=await rpc('tkmphone_transferNumberSigningHash',[n,to]);
        const signature=await signPhone(from,password.value,digest,true);
        const result=await rpc('tkmphone_transferNumber',[n,to,signature]);
        out.innerHTML='';out.appendChild(jsonView(result));toast('Number moved to PQ. Register your device next.','ok');
      } finally {password.value='';}
    }).catch(()=>{});
    container.body.appendChild(submit);container.body.appendChild(out);
  }

  function toHex(str) {
    const hex = Array.from(new TextEncoder().encode(str), b => b.toString(16).padStart(2, '0')).join('');
    return '0x' + hex;
  }

  function randomBytesHex(n) {
    const arr = new Uint8Array(n);
    window.crypto.getRandomValues(arr);
    return Array.from(arr).map((b) => b.toString(16).padStart(2, '0')).join('');
  }
})();