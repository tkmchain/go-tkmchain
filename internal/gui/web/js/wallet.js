/* Wallet — accounts, balances, transfers, PQ migration. */
(function () {
  'use strict';
  const { rpc, register, el, card, stat, table, fmtHash, fmtTKM, live, runAction, toast, jsonView, empty } = window.GUI;

  let fieldID = 0;
  let accountCache = null, accountCacheTime = 0;
  async function walletAccounts() { if(accountCache && Date.now()-accountCacheTime<3000) return accountCache; accountCache=await rpc('eth_accounts',[]);accountCacheTime=Date.now();return accountCache; }
  document.addEventListener('visibilitychange',()=>{if(document.hidden) document.querySelectorAll('#view input[type=password],#view .recovery-words').forEach(input=>input.value='');});
  const section = {
    id: 'wallet',
    label: 'Wallet',
    icon: '◈',
    async render(view) {
      const intro = el('div', {class: 'wallet-intro'}, [
        el('div', {}, [el('span', {class: 'eyebrow', text: 'YOUR PERSONAL WALLET'}), el('h2', {text: 'Your TKM, in your control.'}), el('p', {text: 'Manage your accounts, receive funds and send securely from your own node.'})]),
        el('span', {class: 'wallet-network', text: 'TKM · Local wallet'})
      ]);
      view.appendChild(intro);
      const tabs = el('div', {class: 'wallet-tabs', role: 'tablist', 'aria-label': 'Wallet actions'});
      view.appendChild(tabs);
      const definitions = [['Accounts', 'Your balances and receiving addresses'], ['Send TKM', 'Review your transfer before sending'], ['Create / Import', 'Add an account to this device'], ['Account migration', 'Move a legacy account to post-quantum signing'], ['Receive & backup', 'Your shielded address and wallet recovery'], ['Activity', 'Recent transfers made on this device']];
      definitions.forEach(([title, hint], index) => {
        const panel = card(title, hint);
        panel.id = 'wallet-panel-' + index;
        panel.setAttribute('role', 'tabpanel');
        panel.setAttribute('aria-labelledby', 'wallet-tab-' + index);
        panel.hidden = index !== 0;
        const button = el('button', {type: 'button', id: 'wallet-tab-' + index, role: 'tab', 'aria-controls': panel.id, 'aria-selected': String(index === 0), tabindex: index === 0 ? '0' : '-1', class: index === 0 ? 'active' : '', text: ['Overview', 'Send', 'Add wallet', 'Migration', 'Receive', 'Activity'][index]});
        button.onclick = () => {
          tabs.querySelectorAll('button').forEach((tab, i) => { tab.classList.toggle('active', i === index); tab.setAttribute('aria-selected', String(i === index)); tab.tabIndex = i === index ? 0 : -1; });
          view.querySelectorAll('.card').forEach((card, i) => { card.hidden = i !== index; if(!card.hidden && i === 5) renderActivity(card); if(card.hidden && card.clearSecrets) card.clearSecrets(); });
        };
        button.onkeydown = (event) => {
          const keys = {ArrowRight: (index + 1) % definitions.length, ArrowLeft: (index + definitions.length - 1) % definitions.length, Home: 0, End: definitions.length - 1};
          if (keys[event.key] === undefined) return;
          event.preventDefault();
          const target = tabs.children[keys[event.key]]; target.click(); target.focus();
        };
        tabs.appendChild(button);
        view.appendChild(panel);
      });
      await live(view);
      await refreshWallet(view);
    },
    async refresh(view) {
      await renderAccounts(view.querySelector('.card'));
    },
  };
  register(section);

  async function refreshWallet(view) {
    const cards = view.querySelectorAll('.card');
    const [accountsCard, sendCard, createCard, migCard, viewCard] = cards;
    await walletAccounts();
    await Promise.all([renderAccounts(accountsCard),renderSend(sendCard),renderCreate(createCard),renderMigration(migCard),renderViewHelpers(viewCard),renderActivity(cards[5])]);
  }

  async function renderAccounts(container) {
    container.body.innerHTML = '';
    let accounts = [];
    try { accounts = await walletAccounts(); } catch (e) {
      container.body.appendChild(el('div', { class: 'dim', text: 'eth_accounts failed: ' + e.message }));
      return;
    }
    if (!accounts.length) {
      container.body.appendChild(empty('Your wallet starts here. Choose Add wallet to create a secure account or import an existing one.'));
      return;
    }
    let algos = {};
    try { algos = (await rpc('tkm_accountAlgorithms', [])) || {}; } catch (e) {}

    const rows = [];
    let total = 0n;
    let balanceUnavailable = false;
    const balances=await Promise.all(accounts.map(addr=>rpc('eth_getBalance',[addr,'latest']).catch(()=>null)));
    for (const [accountIndex,addr] of accounts.entries()) {
      let bal = '—';
      try { const amount = balances[accountIndex]; if(amount===null) throw new Error('unavailable'); total += BigInt(amount); bal = fmtTKM(amount, 4) + ' TKM'; } catch (e) { bal = 'Balance unavailable'; balanceUnavailable = true; }
      const algo = algos[addr] || algos[addr.toLowerCase()] || 'unknown';
      const isPQ = String(algo).toLowerCase() === 'ml-dsa-87';
      rows.push([
        el('td', {}, [el('span', {class:'account-address', text:addr}), el('button', {class:'btn secondary copy-address', type:'button', text:'Copy', onclick:async () => { try { await navigator.clipboard.writeText(addr); toast('Address copied.', 'ok'); } catch (e) { toast('Select and copy the address manually.', 'err'); } }})]),
        el('td', { text: bal, class: 'r' }),
        el('td', {}, [el('span', { class: 'badge ' + (isPQ ? 'info' : 'ok'), text: algo })]),
        el('td', { text: isPQ ? '✓ PQ' : '—' }),
      ]);
    }
    container.body.appendChild(el('div', {class:'balance-hero'}, [el('span', {class:'eyebrow', text:'TOTAL PUBLIC BALANCE'}), el('div', {class:'balance-amount', text:balanceUnavailable ? 'Balance unavailable' : fmtTKM(total, 4) + ' TKM'}), el('p', {text:accounts.length + ' account' + (accounts.length === 1 ? '' : 's') + ' on this device · Shielded funds are separate'})]));
    container.body.appendChild(table(['Account', 'Public balance', 'Signing', 'Protection'], rows));
    container.body.appendChild(el('div', { class: 'dim', style: 'margin-top:10px', text: 'Accounts live in the node keystore. Balances are shown in TKM (18 decimals).' }));
  }

  async function renderSend(container) {
    container.body.innerHTML = '';
    const accounts = await walletAccounts();
    const from = el('select', {class:'txt', id:'shield-send-from'});
    for (const address of accounts) from.appendChild(el('option',{value:address,text:address}));
    const to = el('textarea',{class:'txt',id:'shield-send-to',placeholder:'tkmshield2.…',spellcheck:'false',rows:'3'});
    const amount = el('input',{class:'txt',id:'shield-send-amount',inputmode:'decimal',placeholder:'0.00'});
    const pass = el('input',{class:'txt',id:'shield-send-password',type:'password',autocomplete:'current-password'});
    const status = el('p',{class:'wallet-status',role:'status'});
    const review = el('div',{class:'transfer-review'});review.hidden=true;
    const send = el('button',{class:'btn gold',type:'button',text:'Review transfer'});
    const confirm = el('button',{class:'btn gold',type:'button',text:'Confirm & send'});
    const cancel = el('button',{class:'btn secondary',type:'button',text:'Cancel'});
    let prepared=null;
    const row=(label,input)=>el('div',{class:'form-row'},[el('label',{for:input.id,text:label}),input]);
    container.body.append(row('From · PQ account',from),row('To · shield2 receiving address',to),row('Amount (TKM)',amount),row('Wallet password',pass),send,review,status);
    const invalidate=()=>{prepared=null;review.hidden=true;};[from,to,amount,pass].forEach(input=>input.addEventListener('input',invalidate));
    send.onclick=async()=>{
      try {
        const engine=await GUI.engine();const recipient=engine.validateRecipient(to.value);
        const wei=tkmToWei(amount.value.trim());
        if(!from.value || !pass.value || wei===null || BigInt(wei)<=0n) throw new Error('Choose a wallet, enter a positive amount and its password.');
        const source=await rpc('tkm_pqShieldedPaymentCode',[from.value]);engine.validateRecipient(source);
        prepared={from:source,recipient_address:to.value.trim(),amount:amount.value.trim(),account:from.value};
        review.replaceChildren(el('h3',{text:'Review shielded transfer'}),el('p',{text:prepared.amount+' TKM · network fee additional'}),el('p',{class:'mono',text:'Recipient PQ identity: '+recipient.address}),el('p',{class:'mono',text:prepared.recipient_address}),el('div',{class:'btn-row'},[confirm,cancel]));review.hidden=false;status.textContent='Check the complete receiving code before confirming.';
      } catch(e){status.textContent=e.message;}
    };
    cancel.onclick=invalidate;
    confirm.onclick=async()=>{
      if(!prepared)return;const intent={...prepared};const password=pass.value;
      [send,confirm,cancel,from,to,amount,pass].forEach(x=>x.disabled=true);
      try {
        const engine=await GUI.engine();status.textContent='Unlocking the local encrypted wallet…';
        const encrypted=await rpc('tkm_exportPQAccount',[intent.account,password,password]);
        const hashes=await engine.sendTKM({keystore:engine.keyfileFromHex(encrypted),password,intent,
          rpcURL:location.origin+'/rpc',rpcToken:GUI.rpcToken(),proverURL:location.origin+'/prover',
          proverFetch:(url,options)=>fetch(url,{...options,headers:{...options.headers,'X-GUI-Token':GUI.rpcToken()}}),
          onProgress:message=>status.textContent=message,beforePart:async()=>intent,
          onSubmitted:async(_,hash)=>{status.textContent='Submitted: '+hash;recordTransfer(hash,intent);}});
        status.textContent='Submitted. Track confirmation in Activity: '+hashes.join(', ');invalidate();
      } catch(e){status.textContent=e.message;}
      finally {pass.value='';[send,confirm,cancel,from,to,amount,pass].forEach(x=>x.disabled=false);}
    };
  }

  function recordTransfer(hash,intent) {
    const key='tkm-wallet-transfers-v1';let rows=[];
    try{rows=JSON.parse(localStorage.getItem(key)||'[]');}catch(e){}
    rows.unshift({hash,amount:intent.amount,account:intent.account,created:new Date().toISOString()});
    localStorage.setItem(key,JSON.stringify(rows.slice(0,100)));
  }

  async function renderCreate(container) {
    container.body.innerHTML='';
    const row=(label,input)=>{input.id='wallet-field-'+(++fieldID);return el('div',{class:'form-row'},[el('label',{for:input.id,text:label}),input]);};
    const password=el('input',{class:'txt',type:'password',autocomplete:'new-password'});
    const repeat=el('input',{class:'txt',type:'password',autocomplete:'new-password'});
    const phrase=el('textarea',{class:'txt recovery-words',autocomplete:'off',autocapitalize:'none',spellcheck:'false',rows:'5',placeholder:'Enter your 24 TKM PQ recovery words'});
    const generate=el('button',{class:'btn gold',text:'Generate recovery phrase'});
    const restore=el('button',{class:'btn secondary',text:'Restore existing wallet'});
    const save=el('button',{class:'btn gold',text:'Create wallet from saved phrase'});save.hidden=true;
    const acknowledgement=el('input',{type:'checkbox'});
    const ackRow=el('label',{class:'recovery-check'},[acknowledgement,' I saved these words privately. They recover this wallet.']);ackRow.hidden=true;
    const challenge=el('input',{class:'txt',placeholder:'Enter recovery word 6',autocomplete:'off'});challenge.hidden=true;
    const hide=el('button',{class:'btn secondary',text:'Clear recovery words'});
    const status=el('p',{class:'wallet-status',role:'status'});
    let generated='';
    const clear=()=>{phrase.value='';generated='';challenge.value='';acknowledgement.checked=false;save.hidden=true;ackRow.hidden=true;challenge.hidden=true;};
    hide.onclick=clear;
    container.body.append(el('p',{class:'dim',text:'A TKM PQ wallet uses 24 recovery words. These recover its post-quantum identity and shield2 receiving address. Keep them offline; anyone with the words can spend your funds.'}),row('New wallet password',password),row('Confirm password',repeat),el('div',{class:'btn-row'},[generate]),row('TKM PQ recovery phrase · 24 words',phrase),ackRow,challenge,el('div',{class:'btn-row'},[save,restore,hide]),status);
    generate.onclick=async()=>{try{generated=(await GUI.engine()).newRecoveryPhrase();phrase.value=generated;save.hidden=false;ackRow.hidden=false;challenge.hidden=false;acknowledgement.checked=false;status.textContent='Write down all 24 words in order, then confirm word 6.';}catch(e){status.textContent=e.message;}};
    const importPhrase=async creating=>{
      if(password.value.length<10 || password.value!==repeat.value){status.textContent='Use a password of at least 10 characters and confirm it.';return;}
      if(creating && (!acknowledgement.checked || phrase.value!==generated || challenge.value.trim().toLowerCase()!==generated.split(' ')[5])){status.textContent='Confirm your saved phrase and enter the correct sixth word.';return;}
      [generate,save,restore].forEach(x=>x.disabled=true);
      try {const engine=await GUI.engine();let seed=engine.seedFromPhrase(phrase.value);const address=await rpc('tkm_importPQSeedWithPassphrase',[seed,password.value]);seed='';accountCache=null;clear();password.value='';repeat.value='';status.textContent='Wallet ready: '+address;await renderAccounts(document.querySelector('#wallet-panel-0'));await renderSend(document.querySelector('#wallet-panel-1'));await renderViewHelpers(document.querySelector('#wallet-panel-4'));}
      catch(e){status.textContent=e.message;}finally{[generate,save,restore].forEach(x=>x.disabled=false);}
    };
    save.onclick=()=>importPhrase(true);restore.onclick=()=>importPhrase(false);
    // A phrase is never persisted. Clear sensitive input when this screen is hidden.
    container.clearSecrets=()=>{clear();password.value='';repeat.value='';};
  }

  async function renderMigration(container) {
    container.body.innerHTML = '';
    const mkRow = (label, input) => { input.id = 'wallet-field-' + (++fieldID); return el('div', { class: 'form-row' }, [el('label', { text: label, for: input.id }), input]); };

    const addrInput = el('input', { class: 'txt', placeholder: '0x account to migrate' });
    const passInput = el('input', { class: 'txt', placeholder: 'passphrase', type: 'password' });
    const prepBtn = el('button', { class: 'btn secondary', text: 'Prepare migration' });
    const out = el('pre', { class: 'json-view' });

    prepBtn.onclick = () => {
      const addr = addrInput.value.trim();
      const pass = passInput.value;
      if (!/^0x[0-9a-fA-F]{40}$/.test(addr) || !pass) { toast('Enter a valid account and passphrase.', 'err'); return; }
      runAction(prepBtn, async () => {
        const res = await rpc('tkm_preparePQMigrationWithPassphrase', [addr, pass]);
        out.textContent = JSON.stringify(res, null, 2);
        toast('Migration prepared — review details; PK keys stay in the wallet.');
      });
    };

    const migBtn = el('button', { class: 'btn gold', text: 'Auto-migrate', onclick: () => {
      const addr = addrInput.value.trim();
      const pass = passInput.value;
      if (!/^0x[0-9a-fA-F]{40}$/.test(addr) || !pass) { toast('Enter a valid account and passphrase.', 'err'); return; }
      runAction(migBtn, async () => {
        const args = { from: addr };
        const res = await rpc('tkm_autoMigrateToPQWithPassphrase', [args, pass]);
        out.textContent = JSON.stringify(res, null, 2);
        toast('PQ migration submitted.');
      });
    } });

    container.body.appendChild(el('div', { class: 'dim', text: 'After the quantum-resistance fork, user transactions are ML-DSA signed. Migrate a legacy account to PQ with these helpers (the passphrase never leaves the node keystore flow).' }));
    container.body.appendChild(mkRow('Legacy account', addrInput));
    container.body.appendChild(mkRow('Passphrase', passInput));
    container.body.appendChild(el('div', { class: 'btn-row' }, [prepBtn, migBtn]));
    container.body.appendChild(out);
  }

  async function renderViewHelpers(container) {
    container.body.innerHTML = '';
    const accounts = await walletAccounts();
    if (!accounts.length) { container.body.appendChild(empty('Create or import a wallet to receive TKM.')); return; }
    const select = el('select', {class:'txt', id:'receive-account'});
    accounts.forEach(address => select.appendChild(el('option', {value:address, text:address})));
    const output = el('textarea', {class:'txt receive-code', readonly:'readonly', 'aria-label':'Shielded receiving address', rows:'5'});
    const status = el('p', {class:'dim', role:'status'});
    const copy = el('button', {class:'btn', type:'button', text:'Copy receiving address'});
    copy.disabled = true;
    copy.onclick = async () => {
      try { await navigator.clipboard.writeText(output.value); toast('Receiving address copied.', 'ok'); }
      catch (e) { output.focus(); output.select(); toast('Address selected. Use Copy on your device.'); }
    };
    container.body.appendChild(el('div', {class:'form-row'}, [el('label', {for:'receive-account', text:'Receiving account'}), select]));
    container.body.appendChild(el('p', {class:'dim', text:'Use this shielded payment code to receive TKM into the selected account.'}));
    container.body.appendChild(output); container.body.appendChild(status); container.body.appendChild(copy);
    const load = async () => {
      const address = select.value;
      output.value = ''; copy.disabled = true; status.textContent = 'Loading receiving address…';
      try {
        const code = await rpc('tkm_pqShieldedPaymentCode', [address]);
        if (select.value !== address) return;
        if (typeof code !== 'string' || !code.startsWith('tkmshield')) throw new Error('This account has no shielded receiving address.');
        output.value = code; copy.disabled = false; status.textContent = 'TKM network only. Share this address with the sender.';
      } catch (e) { if (select.value === address) status.textContent = e.message + ' Use a post-quantum wallet or check account migration.'; }
    };
    select.onchange = load;
    await load();
    const pass=el('input',{class:'txt',type:'password',id:'backup-wallet-password',autocomplete:'current-password'});
    const backup=el('button',{class:'btn secondary',text:'Reveal recovery phrase'});
    const scan=el('button',{class:'btn gold',text:'Scan shielded balance'});
    const phrase=el('textarea',{class:'txt recovery-words',readonly:'readonly','aria-label':'Recovery phrase',rows:'5'});phrase.hidden=true;
    const hide=el('button',{class:'btn secondary',text:'Hide recovery phrase'});hide.hidden=true;
    const clear=()=>{phrase.value='';phrase.hidden=true;hide.hidden=true;pass.value='';};hide.onclick=clear;container.clearSecrets=clear;
    select.addEventListener('change',clear);
    container.body.append(el('hr'),el('h3',{text:'Private balance & recovery'}),el('p',{class:'dim',text:'Unlock only when needed. Your phrase grants full access to this wallet; store it privately offline.'}),el('div',{class:'form-row'},[el('label',{for:pass.id,text:'Wallet password'}),pass]),el('div',{class:'btn-row'},[scan,backup,hide]),phrase);
    const unlock=async action=>{
      if(!pass.value){status.textContent='Enter your wallet password.';return;}
      backup.disabled=scan.disabled=true;const password=pass.value;
      try{const engine=await GUI.engine();const encrypted=await rpc('tkm_exportPQAccount',[select.value,password,password]);const keyfile=engine.keyfileFromHex(encrypted);await action(engine,keyfile,password);}
      catch(e){status.textContent=e.message;}finally{pass.value='';backup.disabled=scan.disabled=false;}
    };
    backup.onclick=()=>unlock(async(engine,keyfile,password)=>{phrase.value=await engine.recoveryPhraseForKeyfile(keyfile,password);phrase.hidden=false;hide.hidden=false;status.textContent='24-word TKM PQ recovery. Never share this phrase. Hide it when finished.';});
    scan.onclick=()=>unlock(async(engine,keyfile,password)=>{const balance=await engine.scanWallet({keystore:keyfile,password,rpcURL:location.origin+'/rpc',rpcToken:GUI.rpcToken(),onProgress:(block,latest)=>status.textContent=`Scanning shielded notes: ${block} / ${latest}`});status.textContent='Spendable shielded balance: '+balance+' TKM';});
  }

  async function renderActivity(container) {
    if(!container)return;container.body.innerHTML='';
    let rows=[];try{rows=JSON.parse(localStorage.getItem('tkm-wallet-transfers-v1')||'[]');}catch(e){}
    const refresh=el('button',{class:'btn secondary',text:'Refresh confirmations',onclick:()=>renderActivity(container)});container.body.appendChild(refresh);
    if(!rows.length){container.body.appendChild(empty('No transfers submitted from this device yet.'));return;}
    const list=el('div');container.body.appendChild(list);
    await Promise.all(rows.slice(0,25).map(async row=>{
      const item=el('div',{class:'stat'},[el('strong',{text:row.amount+' TKM'}),el('p',{class:'mono',text:row.hash}),el('small',{class:'dim',text:row.created})]);list.appendChild(item);
      let message='Awaiting confirmation';try{const receipt=await rpc('eth_getTransactionReceipt',[row.hash]);if(receipt)message=BigInt(receipt.status)===1n?'Confirmed':'Reverted';}catch(e){message='Confirmation unavailable';}
      item.appendChild(el('p',{text:message}));
    }));
  }

  /* Convert a decimal TKM string ("5.5") to a 0x wei quantity (18 decimals). */
  function tkmToWei(str) {
    if (!/^\d+(\.\d{1,18})?$/.test(str)) return null;
    const [whole, frac = ''] = str.split('.');
    const fracPadded = (frac + '000000000000000000').slice(0, 18);
    const num = BigInt((whole + fracPadded).replace(/^0+(?=\d)/, '') || '0');
    return '0x' + num.toString(16);
  }

  function hexEncode(str) {
    let out = '';
    for (let i = 0; i < str.length; i++) out += str.charCodeAt(i).toString(16).padStart(2, '0');
    return out;
  }
})();