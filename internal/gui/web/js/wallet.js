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
    if((await rpc("tkmprivacy_shieldedV3Status",[]).catch(()=>({active:false}))).active)return renderShield3Send(container);
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
    const shield3=Boolean(intent.recipient_address?.startsWith('tkmshield3.'));
    rows.unshift({hash,amount:shield3?null:intent.amount,shield3,requestId:intent.requestId,stamping:Boolean(intent.stamping),account:intent.account,created:new Date().toISOString()});
    localStorage.setItem(key,JSON.stringify(rows.slice(0,100)));
  }

  async function renderCreate(container) {
    container.body.innerHTML='';
    const row=(label,input)=>{input.id='wallet-field-'+(++fieldID);return el('div',{class:'form-row'},[el('label',{for:input.id,text:label}),input]);};
    const shield3Active=(await rpc('tkmprivacy_shieldedV3Status',[]).catch(()=>({active:false}))).active;
    const stampName=el('input',{class:'txt',maxlength:'120',autocomplete:'name'}),stampCountry=el('input',{class:'txt',maxlength:'80',autocomplete:'country-name'});
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
    container.body.append(el('p',{class:'dim',text:'A TKM PQ wallet uses 24 recovery words. These recover its post-quantum identity and note keys. Keep an encrypted backup to preserve the original private stamp. Keep them offline; anyone with the words can spend your funds.'}),row('New wallet password',password),row('Confirm password',repeat),el('div',{class:'btn-row'},[generate]),row('TKM PQ recovery phrase · 24 words',phrase),ackRow,challenge,el('div',{class:'btn-row'},[save,restore,hide]),status);
    if(shield3Active){container.body.prepend(el('p',{class:'dim',text:'Create your private name/country stamp before adding this Shield3 wallet. Only its stamp key can reveal these labels.'}),row('Private stamp · name',stampName),row('Private stamp · country',stampCountry));}
    generate.onclick=async()=>{try{if(shield3Active&&(!stampName.value.trim()||!stampCountry.value.trim()))throw Error('Enter your private stamp name and country first.');generated=(await GUI.engine()).newRecoveryPhrase();phrase.value=generated;save.hidden=false;ackRow.hidden=false;challenge.hidden=false;acknowledgement.checked=false;status.textContent='Write down all 24 words in order, then confirm word 6.';}catch(e){status.textContent=e.message;}};
    const importPhrase=async creating=>{
      if(password.value.length<10 || password.value!==repeat.value){status.textContent='Use a password of at least 10 characters and confirm it.';return;}
      if(creating && (!acknowledgement.checked || phrase.value!==generated || challenge.value.trim().toLowerCase()!==generated.split(' ')[5])){status.textContent='Confirm your saved phrase and enter the correct sixth word.';return;}
      [generate,save,restore].forEach(x=>x.disabled=true);
      try {const engine=await GUI.engine();let seed=engine.seedFromPhrase(phrase.value);const address=await rpc(shield3Active?'tkm_importStampedPQSeedWithPassphrase':'tkm_importPQSeedWithPassphrase',shield3Active?[seed,password.value,stampName.value.trim(),stampCountry.value.trim()]:[seed,password.value]);seed='';accountCache=null;clear();password.value='';repeat.value='';status.textContent=(shield3Active?'Private stamp created. Register it on chain in Receive before sending: ':'Wallet ready: ')+address;await renderAccounts(document.querySelector('#wallet-panel-0'));await renderSend(document.querySelector('#wallet-panel-1'));await renderViewHelpers(document.querySelector('#wallet-panel-4'));}
      catch(e){status.textContent=e.message;}finally{[generate,save,restore].forEach(x=>x.disabled=false);}
    };
    save.onclick=()=>importPhrase(true);restore.onclick=()=>importPhrase(false);
    const backupFile=el('input',{class:'txt',type:'file',accept:'.json,application/json'}),backupPassword=el('input',{class:'txt',type:'password',autocomplete:'current-password'}),importBackup=el('button',{class:'btn secondary',text:'Restore encrypted wallet backup'});
    importBackup.onclick=async()=>{importBackup.disabled=true;try{const file=backupFile.files[0];if(!file||file.size>512*1024)throw Error('Choose an encrypted PQ keyfile up to 512 KiB.');if(password.value.length<10||password.value!==repeat.value)throw Error('Choose and confirm a new password of at least 10 characters.');const bytes=new TextEncoder().encode(await file.text());const hex='0x'+Array.from(bytes,b=>b.toString(16).padStart(2,'0')).join('');const address=await rpc('tkm_importPQBackupWithPassphrase',[hex,backupPassword.value,password.value]);accountCache=null;clear();password.value='';repeat.value='';backupFile.value='';status.textContent='Original wallet and stamp restored: '+address;await renderAccounts(document.querySelector('#wallet-panel-0'));await renderSend(document.querySelector('#wallet-panel-1'));await renderViewHelpers(document.querySelector('#wallet-panel-4'));}catch(e){status.textContent=e.message;}finally{backupPassword.value='';importBackup.disabled=false;}};
    container.body.append(el('hr'),el('h3',{text:'Restore encrypted backup'}),el('p',{class:'dim',text:'Restore the encrypted JSON keyfile to preserve your original private stamp. Enter the backup password and choose the new wallet password above.'}),row('Encrypted PQ backup',backupFile),row('Backup password',backupPassword),importBackup);
    // A phrase is never persisted. Clear sensitive input when this screen is hidden.
    container.clearSecrets=()=>{clear();password.value='';repeat.value='';backupPassword.value='';backupFile.value='';};
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
    if((await rpc("tkmprivacy_shieldedV3Status",[]).catch(()=>({active:false}))).active)return renderShield3Receive(container);
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

  async function shield3Unlock(account,password) {
    if(!account||!password)throw Error('Choose your wallet and enter its password.');
    const engine=await GUI.engine();const encrypted=await rpc('tkm_exportPQAccount',[account,password,password]);
    return {engine,options:{keystore:engine.keyfileFromHex(encrypted),password,rpcToken:GUI.rpcToken(),rpcURL:location.origin+'/rpc'}};
  }

  function consensusStampGate(container,select,password,controls) {
    const name=el('input',{class:'txt',id:'stamp-name-'+(++fieldID),autocomplete:'name',maxlength:'120'}),country=el('input',{class:'txt',id:'stamp-country-'+(++fieldID),autocomplete:'country-name',maxlength:'80'});
    const register=el('button',{class:'btn gold',text:'Stamp address · register on chain'}),refresh=el('button',{class:'btn secondary',text:'Check stamp confirmation'}),state=el('p',{class:'wallet-status',role:'status'});
    const section=el('section',{class:'wallet-stamp-step'},[el('h3',{text:'First: stamp your address'}),el('p',{class:'dim',text:'Create your private name/country stamp, register it, and wait for confirmation before sending. Existing stamped backups keep their original stamp. Registration transfers no amount and uses normal gas fees.'}),el('div',{class:'form-row'},[el('label',{for:name.id,text:'Private name'}),name]),el('div',{class:'form-row'},[el('label',{for:country.id,text:'Private country'}),country]),el('div',{class:'btn-row'},[register,refresh]),state]);
    container.body.appendChild(section);
    let busy=false;
    const update=async()=>{
      controls.forEach(button=>button.disabled=true);
      try {const address=select.value;const stamp=await rpc('tkmprivacy_antarticalStamp',[address]);if(address!==select.value)return;register.disabled=busy||stamp.registered;controls.forEach(button=>button.disabled=busy||!stamp.registered);state.textContent=stamp.registered?'Stamp confirmed: '+stamp.transactionHash:'Stamp required. Sending stays disabled until registration confirms.';if(!stamp.registered){try{const draft=JSON.parse(localStorage.getItem('tkm-stamp-drafts-v1')||'{}')[address];if(draft?.hash)state.textContent='Stamp unconfirmed: '+draft.hash+'. Sending stays disabled.';}catch(e){}}name.disabled=country.disabled=stamp.registered;}
      catch(e){state.textContent='Cannot verify stamp: '+e.message;register.disabled=busy;}
    };
    refresh.onclick=update;
    register.onclick=async()=>{
      const account=select.value,secret=password.value;busy=true;register.disabled=true;[select,password,name,country].forEach(input=>input.disabled=true);controls.forEach(button=>button.disabled=true);
      try{
        if(!secret)throw Error('Enter the wallet password above.');
        let unlocked=await shield3Unlock(account,secret);
        if(!unlocked.options.keystore.shield3Stamp){if(!name.value.trim()||!country.value.trim())throw Error('Enter your private name and country first.');await rpc('tkm_stampPQAccountWithPassphrase',[account,secret,name.value.trim(),country.value.trim()]);unlocked=await shield3Unlock(account,secret);}
        let drafts={};try{drafts=JSON.parse(localStorage.getItem('tkm-stamp-drafts-v1')||'{}')}catch(e){}
        const requestId=drafts[account]?.requestId||crypto.randomUUID();drafts[account]={...drafts[account],requestId};localStorage.setItem('tkm-stamp-drafts-v1',JSON.stringify(drafts));
        state.textContent='Building stamp ownership proof…';const result=await unlocked.engine.shield3RegisterStamp({...unlocked.options,requestId});drafts[account].hash=result.transactionHash;localStorage.setItem('tkm-stamp-drafts-v1',JSON.stringify(drafts));recordTransfer(result.transactionHash,{account,stamping:true});state.textContent='Stamp unconfirmed: '+result.transactionHash+'. Check confirmation before sending.';
      }catch(e){state.textContent=e.message;}finally{password.value='';busy=false;register.disabled=false;[select,password,name,country].forEach(input=>input.disabled=false);}
    };
    const requestCode=el('textarea',{class:'txt',rows:'3',readonly:'readonly','aria-label':'My stamp sponsorship request'});
    const getCode=el('button',{class:'btn secondary',text:'Create my stamp sponsorship request'});
    const beneficiaryCode=el('textarea',{class:'txt',rows:'3',placeholder:'Paste the new wallet’s stamp request (tkmshield3.…)',spellcheck:'false','aria-label':'Beneficiary stamp request'});
    const offerButton=el('button',{class:'btn secondary',text:'1. Sponsor: create fee offer'});
    const packetInput=el('textarea',{class:'txt',rows:'3',placeholder:'Paste a fee offer or authorized packet, or load its JSON file',spellcheck:'false','aria-label':'Stamp sponsorship packet'});
    const file=el('input',{type:'file',accept:'.json,application/json','aria-label':'Load stamp sponsorship packet'});
    const authorize=el('button',{class:'btn secondary',text:'2. New wallet: authorize my stamp'});
    const reviewSponsor=el('button',{class:'btn secondary',text:'3. Sponsor: review fee'});
    const submitSponsor=el('button',{class:'btn gold',text:'Confirm sponsor fee & submit',disabled:'disabled'});
    const download=el('button',{class:'btn secondary',text:'Download packet JSON',disabled:'disabled'});
    const sponsorState=el('p',{class:'wallet-status',role:'status'});
    const details=el('details',{class:'wallet-stamp-sponsorship'},[el('summary',{text:'Register with a sponsor · no TKM needed in the new wallet'}),el('p',{class:'dim',text:'The sponsor must already have a confirmed stamp and public TKM for gas. Exchange the request, fee offer and authorized packet. Offers expire after one hour and reserve the sponsor’s current nonce; changing its transactions requires a new offer. Packets contain encrypted labels and public authorization data, never private keys.'}),getCode,requestCode,beneficiaryCode,offerButton,packetInput,file,el('div',{class:'btn-row'},[authorize,reviewSponsor,submitSponsor,download]),sponsorState]);
    section.appendChild(details);
    let exportedPacket=null,reviewedPacket=null;
    const sponsorshipButtons=[getCode,offerButton,authorize,reviewSponsor,submitSponsor];
    const readPacket=()=>{const value=JSON.parse(packetInput.value);if(typeof value.transaction!=='string'||value.transaction.length>17_000_000||!/^0x(?:[0-9a-fA-F]{2})+$/.test(value.transaction))throw Error('Invalid sponsorship packet.');return value.transaction;};
    const resetReview=()=>{reviewedPacket=null;submitSponsor.disabled=true;};
    packetInput.addEventListener('input',resetReview);
    file.onchange=async()=>{try{if(!file.files[0])return;if(file.files[0].size>20*1024*1024)throw Error('Sponsorship file is too large.');packetInput.value=await file.files[0].text();readPacket();resetReview();sponsorState.textContent='Packet loaded. Choose the matching wallet and the next step.';}catch(e){sponsorState.textContent=e.message;}};
    download.onclick=()=>{if(!exportedPacket)return;const url=URL.createObjectURL(new Blob([JSON.stringify(exportedPacket)],{type:'application/json'}));const link=el('a',{href:url,download:'tkm-stamp-sponsorship.json'});link.click();setTimeout(()=>URL.revokeObjectURL(url),1000);};
    const sponsorshipAction=async(stage)=>{
      const account=select.value,secret=password.value;busy=true;resetReview();sponsorshipButtons.forEach(button=>button.disabled=true);[select,password,name,country,packetInput,file,beneficiaryCode,register].forEach(input=>input.disabled=true);controls.forEach(button=>button.disabled=true);
      try{
        let unlocked=await shield3Unlock(account,secret);
        if(stage==='request'&&!unlocked.options.keystore.shield3Stamp){if(!name.value.trim()||!country.value.trim())throw Error('Enter your private name and country first.');await rpc('tkm_stampPQAccountWithPassphrase',[account,secret,name.value.trim(),country.value.trim()]);unlocked=await shield3Unlock(account,secret);}
        if(stage==='request'){const identity=await unlocked.engine.shield3Identity(unlocked.options);requestCode.value=identity.paymentCode;sponsorState.textContent='Share this stamp request with your sponsor. Payments remain disabled until your stamp confirms.';return;}
        const sponsorship=stage==='offer'?undefined:readPacket();
        let requestId;
        if(stage==='submit'){
          const digest=Array.from(new Uint8Array(await crypto.subtle.digest('SHA-256',new TextEncoder().encode(sponsorship)))).map(byte=>byte.toString(16).padStart(2,'0')).join('');
          const key=account+':'+digest;let drafts={};try{drafts=JSON.parse(localStorage.getItem('tkm-stamp-sponsored-drafts-v1')||'{}')}catch(e){}
          requestId=drafts[key]?.requestId||crypto.randomUUID();drafts[key]={...drafts[key],requestId};localStorage.setItem('tkm-stamp-sponsored-drafts-v1',JSON.stringify(drafts));
        }
        sponsorState.textContent=stage==='authorize'?'Building your stamp ownership proof…':'Checking stamp sponsorship…';
        const result=await unlocked.engine.shield3StampSponsorship({...unlocked.options,stage,recipient:beneficiaryCode.value.trim(),sponsorship,requestId});
        if(stage==='submit'){recordTransfer(result.transactionHash,{account,stamping:true});sponsorState.textContent='Sponsored stamp unconfirmed: '+result.transactionHash+'. The new wallet can check stamp confirmation using its address.';return;}
        exportedPacket=result;download.disabled=false;
        if(stage==='review'){reviewedPacket=result.transaction;submitSponsor.disabled=false;sponsorState.textContent='Verified sponsor: '+result.sponsor+' · beneficiary: '+result.beneficiary+' · maximum fee: '+fmtTKM(result.maxFeeWei,18)+' TKM · expires: '+new Date(Number(BigInt(result.validUntil))*1000).toLocaleString()+'. Confirm only if you approve this fee.';}
        else {packetInput.value=JSON.stringify(result);sponsorState.textContent=stage==='offer'?'Download the fee offer and send it to the new wallet for authorization.':'Download the authorized packet and return it to the sponsor. Your balance and nonce are unchanged.';}
      }catch(e){sponsorState.textContent=e.message;}finally{password.value='';busy=false;sponsorshipButtons.forEach(button=>button.disabled=false);[select,password,name,country,packetInput,file,beneficiaryCode,register].forEach(input=>input.disabled=false);submitSponsor.disabled=!reviewedPacket;await update();}
    };
    getCode.onclick=()=>sponsorshipAction('request');offerButton.onclick=()=>sponsorshipAction('offer');authorize.onclick=()=>sponsorshipAction('authorize');reviewSponsor.onclick=()=>sponsorshipAction('review');
    submitSponsor.onclick=()=>{if(!reviewedPacket||readPacket()!==reviewedPacket){resetReview();sponsorState.textContent='Review the current packet before confirming.';return;}sponsorshipAction('submit');};
    select.addEventListener('change',()=>{name.value=country.value='';requestCode.value='';resetReview();update()});
    return update;
  }

  async function renderShield3Send(container) {
    container.body.innerHTML='';const accounts=await walletAccounts();
    const from=el('select',{class:'txt',id:'shield3-from'});accounts.forEach(address=>from.appendChild(el('option',{value:address,text:address})));
    const to=el('textarea',{class:'txt',id:'shield3-to',rows:'4',placeholder:'tkmshield3.…',spellcheck:'false'});
    const amount=el('input',{class:'txt',id:'shield3-amount',inputmode:'decimal',placeholder:'0.00'}),pass=el('input',{class:'txt',id:'shield3-pass',type:'password',autocomplete:'current-password'});
    const status=el('p',{class:'wallet-status',role:'status'}),review=el('div',{class:'transfer-review'});review.hidden=true;
    const send=el('button',{class:'btn gold',text:'Review Shield3 transfer'}),confirm=el('button',{class:'btn gold',text:'Confirm & send'}),cancel=el('button',{class:'btn secondary',text:'Cancel'});
    const row=(text,input)=>el('div',{class:'form-row'},[el('label',{for:input.id,text}),input]);
    container.body.append(row('From · PQ wallet',from),row('Wallet password',pass));
    const updateStamp=consensusStampGate(container,from,pass,[send,confirm]);
    container.body.append(el('p',{class:'dim',text:'Private Shield3 send · maximum 5,000,000 TKM per send. Shield public funds in Receive first.'}),row('To · Shield3 receiving address',to),row('Amount (TKM)',amount),send,review,status);
    await updateStamp();
    let prepared=null;const invalidate=()=>{prepared=null;review.hidden=true;};[from,to,amount,pass].forEach(input=>input.addEventListener('input',invalidate));cancel.onclick=invalidate;
    send.onclick=async()=>{try{const engine=await GUI.engine();engine.validateShield3Amount(amount.value.trim());if(!from.value||!pass.value)throw Error('Choose your wallet and enter its password.');const recipient=await engine.validateShield3Recipient(to.value.trim(),{rpcToken:GUI.rpcToken()});prepared={account:from.value,recipient_address:to.value.trim(),amount:amount.value.trim(),requestId:crypto.randomUUID()};review.replaceChildren(el('h3',{text:'Review private Shield3 transfer'}),el('p',{text:prepared.amount+' TKM · network fee additional'}),el('p',{class:'mono',text:'Recipient: '+recipient.address}),el('div',{class:'btn-row'},[confirm,cancel]));review.hidden=false;status.textContent='Confirm the receiving address and amount.';}catch(e){status.textContent=e.message;}};
    confirm.onclick=async()=>{if(!prepared)return;const intent={...prepared};const password=pass.value;[send,confirm,cancel,from,to,amount,pass].forEach(input=>input.disabled=true);try{status.textContent='Building the Shield3 proof…';const {engine,options}=await shield3Unlock(intent.account,password);const hashes=await engine.sendTKM({...options,intent,requestId:intent.requestId,onProgress:text=>status.textContent=text,onSubmitted:(_,hash)=>recordTransfer(hash,intent)});status.textContent='Unconfirmed: '+hashes.join(', ')+'. Check Activity before retrying.';invalidate();}catch(e){status.textContent=e.message;}finally{pass.value='';[send,confirm,cancel,from,to,amount,pass].forEach(input=>input.disabled=false);await updateStamp();}};
    container.clearSecrets=()=>{pass.value='';invalidate();};
  }
  async function renderShield3Receive(container) {
    container.body.innerHTML='';const select=el('select',{class:'txt',id:'shield3-receive'});(await walletAccounts()).forEach(address=>select.appendChild(el('option',{value:address,text:address})));
    const pass=el('input',{class:'txt',type:'password',id:'shield3-receive-pass',autocomplete:'current-password'}),code=el('textarea',{class:'txt receive-code',readonly:'readonly',rows:'5','aria-label':'Shield3 receiving address'}),status=el('p',{class:'wallet-status',role:'status'});
    const row=(text,input)=>el('div',{class:'form-row'},[el('label',{for:input.id,text}),input]);
    const address=el('button',{class:'btn gold',text:'Unlock receiving address'}),copy=el('button',{class:'btn secondary',text:'Copy address'}),scan=el('button',{class:'btn gold',text:'Scan Shield3 balance'}),backup=el('button',{class:'btn secondary',text:'Save encrypted wallet backup'}),view=el('button',{class:'btn secondary',text:'Reveal viewing and stamp keys'});
    const keys=el('textarea',{class:'txt recovery-words',readonly:'readonly',rows:'6','aria-label':'Private viewing and stamp keys'});keys.hidden=true;
    container.body.append(row('Wallet',select),row('Wallet password',pass));
    const stampControls=[address,copy];const updateStamp=consensusStampGate(container,select,pass,stampControls);
    container.body.append(el('div',{class:'btn-row'},[address,copy,scan,backup,view]),code,keys,status);
    const run=async action=>{[address,scan,backup,view].forEach(button=>button.disabled=true);try{const unlocked=await shield3Unlock(select.value,pass.value);await action(unlocked);}catch(e){status.textContent=e.message;}finally{pass.value='';[address,scan,backup,view].forEach(button=>button.disabled=false);await updateStamp();}};
    address.onclick=()=>run(async({engine,options})=>{code.value=(await engine.shield3Identity(options)).paymentCode;status.textContent='Share this Shield3 receiving address. Its public keys cannot decrypt notes or stamps.';});
    copy.onclick=async()=>{if(!code.value)return;try{await navigator.clipboard.writeText(code.value);toast('Shield3 address copied.','ok');}catch(e){code.focus();code.select();}};
    scan.onclick=()=>run(async({engine,options})=>{status.textContent='Scanning private notes…';const result=await engine.shield3Scan(options);status.textContent='Confirmed spendable Shield3: '+fmtTKM('0x'+BigInt(result.balanceWei).toString(16),8)+' TKM';});
    backup.onclick=()=>run(async({options})=>{const blob=new Blob([JSON.stringify(options.keystore,null,2)],{type:'application/json'});const url=URL.createObjectURL(blob),link=el('a',{href:url,download:'TKM-Shield3-'+select.value+'.json'});link.click();setTimeout(()=>URL.revokeObjectURL(url),1000);status.textContent='Encrypted backup saved. Keep it with your recovery words; it preserves the original private stamp.';});
    view.onclick=()=>run(async({engine,options})=>{keys.value=JSON.stringify(await engine.shield3ViewKeys(options),null,2);keys.hidden=false;status.textContent='Viewing keys disclose note history; the separate stamp key discloses the stamp. These do not grant spending authority.';});
    const amount=el('input',{class:'txt',id:'shield3-fund-amount',inputmode:'decimal',placeholder:'0.00'}),fund=el('button',{class:'btn gold',text:'Shield public funds'});
    fund.onclick=async()=>{try{const text=amount.value.trim(),engine=await GUI.engine();const amountWei=engine.validateShield3Amount(text).toString(),account=select.value;let draft=null;try{draft=JSON.parse(localStorage.getItem('tkm-shield3-funding-draft-v1')||'null');}catch(e){}if(draft?.transactionHash&&await rpc('eth_getTransactionReceipt',[draft.transactionHash]))draft=null;if(!draft||draft.account!==account||draft.amountWei!==amountWei)draft={account,amountWei,requestId:crypto.randomUUID()};if(!confirm('Shield '+text+' TKM from your public balance? This deposit amount is public.'))return;localStorage.setItem('tkm-shield3-funding-draft-v1',JSON.stringify(draft));await run(async({engine,options})=>{status.textContent='Building the funding proof…';const result=await engine.shield3Funds({...options,amount:text,requestId:draft.requestId});draft.transactionHash=result.transactionHash;localStorage.setItem('tkm-shield3-funding-draft-v1',JSON.stringify(draft));recordTransfer(result.transactionHash,{amount:text,account,requestId:draft.requestId});status.textContent='Funding unconfirmed: '+result.transactionHash+'. Scan the balance after confirmation.';});}catch(e){status.textContent=e.message;}};
    stampControls.push(fund);
    container.body.append(el('hr'),el('h3',{text:'Shield public funds'}),el('p',{class:'dim',text:'Move public TKM into a Shield3 note. The deposit amount is public; later private sends hide their amounts. Maximum 5,000,000 TKM.'}),row('Amount to shield (TKM)',amount),fund);
    const migrate=el('button',{class:'btn secondary',text:'Migrate one Shield2 note'});
    migrate.onclick=()=>{if(!confirm('Withdraw one entire Shield2 note to your own public account? Its value becomes public. After confirmation, use Shield public funds to move it into Shield3.'))return;run(async({engine,options})=>{migrate.disabled=true;try{const hash=await engine.migrateShield2Note({...options,onProgress:text=>status.textContent=text,onSubmitted:(hash,amount)=>recordTransfer(hash,{amount,account:select.value})});status.textContent='Migration unconfirmed: '+hash+'. Wait for confirmation before migrating another note or shielding funds.';}finally{migrate.disabled=false;}});};
    stampControls.push(migrate);
    container.body.append(el('hr'),el('h3',{text:'Move Shield2 funds into Shield3'}),el('p',{class:'dim',text:'Migrate each legacy note to your own public balance, wait for confirmation, then shield the confirmed balance. This migration reveals its amount.'}),migrate);
    const clear=()=>{pass.value='';keys.value='';keys.hidden=true;};container.clearSecrets=clear;select.onchange=()=>{clear();code.value='';};await updateStamp();
  }

  async function renderActivity(container) {
    if(!container)return;container.body.innerHTML='';
    let rows=[];try{rows=JSON.parse(localStorage.getItem('tkm-wallet-transfers-v1')||'[]');}catch(e){}
    const refresh=el('button',{class:'btn secondary',text:'Refresh confirmations',onclick:()=>renderActivity(container)});container.body.appendChild(refresh);
    if(!rows.length){container.body.appendChild(empty('No transfers submitted from this device yet.'));return;}
    const list=el('div');container.body.appendChild(list);
    await Promise.all(rows.slice(0,25).map(async row=>{
      const item=el('div',{class:'stat'},[el('strong',{text:row.stamping?'Address stamp registration':row.shield3?'Private Shield3 transfer':row.amount+' TKM'}),el('p',{class:'mono',text:row.hash}),el('small',{class:'dim',text:row.created})]);list.appendChild(item);
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