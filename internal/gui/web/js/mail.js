/* Mail: PQ-owned mailboxes, local encryption and signed shielded actions. */
(function () {
  'use strict';
  const {rpc,register,el,card,stat,fmtTKM,fmtHexNum,fmtTS,empty,jsonView}=window.GUI;
  let cancelReview=null, currentRoot=null;
  const section={id:'mail',label:'Mail',icon:'✉',async render(view){
    currentRoot=view;
    const access=card('Your Mail wallet','Keys stay in your wallet');view.appendChild(access);
    const account=el('select',{class:'txt','aria-label':'Mail owner account'}),password=el('input',{class:'txt',type:'password',autocomplete:'current-password','aria-label':'Mail wallet password'});
    const field=(container,label,input)=>container.appendChild(el('div',{class:'form-row'},[el('label',{},[label,input])]));
    field(access.body,'PQ owner account',account);field(access.body,'Wallet password',password);
    const status=el('div',{'aria-live':'polite',class:'operation-status'}),review=el('div',{class:'transfer-review'});review.hidden=true;
    access.body.appendChild(status);access.body.appendChild(review);
    const accounts=await rpc('tkm_accountAlgorithms',[]);
    for(const [address,algorithm] of Object.entries(accounts||{}))if(algorithm==='ML-DSA-87')account.appendChild(el('option',{value:address,text:address}));
    if(!account.options.length)status.textContent='Create or import a PQ wallet to register, send and decrypt Mail.';
    let running=false;
    const run=async(operation,params)=>{
      if(running)throw Error('A Mail operation is already running.');
      if(!account.value||!password.value)throw Error('Select your PQ account and enter its wallet password.');
      running=true;account.disabled=password.disabled=true;
      const pass=password.value,owner=account.value;
      try {
        const engine=await GUI.engine();status.textContent='Unlocking the encrypted wallet locally…';
        const keyfile=engine.keyfileFromHex(await rpc('tkm_exportPQAccount',[owner,pass,pass]));
        const result=await engine.runMailOperation({keystore:keyfile,password:pass,rpc,operation,params,proverURL:location.origin+'/prover',
          proverFetch:(url,options)=>fetch(url,{...options,headers:{...options.headers,'X-GUI-Token':GUI.rpcToken()}}),
          onProgress:text=>{status.textContent=text;},
          onSubmitted:hash=>{
            const entries=JSON.parse(localStorage.getItem('tkm-mail-transactions-v1')||'[]');
            entries.unshift({hash,owner,operation,time:Date.now()});localStorage.setItem('tkm-mail-transactions-v1',JSON.stringify(entries.slice(0,100)));
            status.textContent='Submitted: '+hash;
          },
          onReview:details=>new Promise(resolve=>{
            if (GUI.current !== section || document.hidden) { resolve(false); return; }
            review.innerHTML='';review.hidden=false;
            review.appendChild(el('p',{text:details.label}));
            review.appendChild(el('p',{text:'Payment: '+details.amount+' TKM · Recipient: '+details.recipient}));
            review.appendChild(el('p',{text:'Maximum total network gas: '+details.maxGas+' TKM across '+details.transactions+' transaction(s), including any self-shielding. Registration and key publication are separate confirmed steps.'}));
            const finish=accepted=>{review.hidden=true;cancelReview=null;resolve(accepted);};cancelReview=()=>finish(false);
            review.appendChild(el('button',{class:'btn',text:'Confirm and sign',onclick:()=>finish(true)}));
            review.appendChild(el('button',{class:'btn secondary',text:'Cancel',onclick:()=>finish(false)}));
          })});
        status.textContent=operation==='decrypt'?'Message decrypted locally.':'Confirmed: '+(result.length?result.join(', '):'Already configured.');
        return result;
      } finally {password.value='';account.disabled=password.disabled=false;running=false;cancelReview?.();}
    };
    const action=(button,work)=>{button.onclick=async()=>{button.disabled=true;try{await work();}catch(error){status.textContent=error.message;}finally{button.disabled=false;}};};
    const registration=card('Register a mailbox','Purchase and publish your encryption key');view.appendChild(registration);
    const username=el('input',{class:'txt',placeholder:'alice','aria-label':'Mailbox username'}),domain=el('input',{class:'txt',value:'tkm','aria-label':'Mailbox domain'});
    field(registration.body,'Username',username);field(registration.body,'Domain',domain);
    const buy=el('button',{class:'btn',text:'Register mailbox'}),publish=el('button',{class:'btn secondary',text:'Publish Mail key'}),lookup=el('button',{class:'btn secondary',text:'Check registration'}),details=el('div');
    const mailbox=()=>username.value.trim().toLowerCase()+'@'+domain.value.trim().toLowerCase().replace(/^@/,'');
    action(buy,()=>run('buy',{username:username.value,domain:domain.value}));
    action(publish,()=>run('publish',{mailbox:mailbox()}));
    action(lookup,async()=>{details.innerHTML='';details.appendChild(jsonView(await rpc('tkmdomain_mailbox',[mailbox()])));});
    registration.body.appendChild(el('div',{class:'btn-row'},[buy,publish,lookup]));registration.body.appendChild(details);
    registration.body.appendChild(el('p',{class:'dim',text:'Your 24-word PQ recovery phrase also restores this wallet’s Mail encryption key. If registration confirms but key publication is interrupted, use Publish Mail key.'}));
    const compose=card('Compose','Encrypted locally before submission');view.appendChild(compose);
    const from=el('input',{class:'txt',placeholder:'alice@tkm','aria-label':'Mail from'}),to=el('input',{class:'txt',placeholder:'bob@tkm','aria-label':'Mail to'}),subject=el('input',{class:'txt','aria-label':'Mail subject'}),body=el('textarea',{class:'txt',rows:6,'aria-label':'Mail message'});
    field(compose.body,'From',from);field(compose.body,'To',to);field(compose.body,'Subject',subject);field(compose.body,'Message',body);
    compose.body.appendChild(el('p',{class:'dim',text:'Both mailboxes must have published encryption keys. Maximum message size: 4,000 UTF-8 bytes including subject.'}));
    const send=el('button',{class:'btn',text:'Encrypt, sign and send'});action(send,async()=>{await run('send',{from:from.value,to:to.value,subject:subject.value,body:body.value});body.value='';subject.value='';});compose.body.appendChild(send);
    for(const inbox of [true,false]) {
      const box=card(inbox?'Inbox':'Sent Mail','Load all pages and decrypt messages');view.appendChild(box);
      const name=el('input',{class:'txt',placeholder:'alice@tkm','aria-label':inbox?'Inbox mailbox':'Sent mailbox'}),load=el('button',{class:'btn secondary',text:'Load messages'}),more=el('button',{class:'btn secondary',text:'Load more'}),out=el('div');more.hidden=true;
      let offset='0x0',loaded='';field(box.body,'Mailbox',name);
      const read=async append=>{
        const mb=name.value.trim().toLowerCase();if(!mb)throw Error('Enter a mailbox.');
        if(!append||mb!==loaded){offset='0x0';out.innerHTML='';}loaded=mb;
        const page=await rpc(inbox?'emailvm_inboxPage':'emailvm_outboxPage',[mb,offset,'0x20']);
        for(const message of page.messages||[]) {
          const item=el('details',{class:'mail-message'}),plain=el('pre',{class:'mail-plaintext'}),decrypt=el('button',{class:'btn secondary',text:'Decrypt message'});
          item.appendChild(el('summary',{text:(inbox?message.from:message.to)+' · '+fmtTS(message.timestamp)}));
          item.appendChild(jsonView(message));item.appendChild(decrypt);item.appendChild(plain);
          action(decrypt,async()=>{plain.textContent=await run('decrypt',{mailbox:mb,message});});out.appendChild(item);
        }
        if(!(page.messages||[]).length&&!append)out.appendChild(empty('No messages.'));
        offset=page.nextOffset;more.hidden=!page.hasMore;
      };
      action(load,()=>read(false));action(more,()=>read(true));box.body.appendChild(load);box.body.appendChild(out);box.body.appendChild(more);
    }
    const activity=card('Mail transactions','Check confirmations before retrying');view.appendChild(activity);
    const refresh=el('button',{class:'btn secondary',text:'Refresh Mail transactions'}),history=el('div');activity.body.appendChild(refresh);activity.body.appendChild(history);
    action(refresh,async()=>{history.innerHTML='';const entries=JSON.parse(localStorage.getItem('tkm-mail-transactions-v1')||'[]');for(const entry of entries.slice(0,25)){const receipt=await rpc('eth_getTransactionReceipt',[entry.hash]);history.appendChild(el('p',{class:'mono',text:entry.operation+' · '+entry.hash+' · '+(receipt?(BigInt(receipt.status)===1n?'confirmed':'reverted'):'pending / unknown')}));}if(!entries.length)history.appendChild(empty('No Mail transactions from this device.'));});
    const network=card('Mail network','Canonical registration state');view.appendChild(network);
    try {const [s,domains]=await Promise.all([rpc('emailvm_status',[]),rpc('tkmdomain_domains',[])]);network.body.appendChild(stat('Status',s.ready?'ready':'indexing'));network.body.appendChild(stat('Indexed block',fmtHexNum(s.indexedBlock)));for(const d of domains||[])network.body.appendChild(el('details',{},[el('summary',{text:'@'+d.name+' · '+fmtHexNum(d.usedUnits)+' / '+fmtHexNum(d.totalUnits)+' mailboxes · '+fmtTKM(d.subscriberPrice)+' TKM'}),jsonView(d)]));}catch(e){network.body.appendChild(empty(e.message));}
  },onHidden(){cancelReview?.();if(currentRoot){for(const input of currentRoot.querySelectorAll('input[type="password"],textarea'))input.value='';for(const plaintext of currentRoot.querySelectorAll('.mail-plaintext'))plaintext.textContent='';}}};
  document.addEventListener('visibilitychange',()=>{if(document.hidden && GUI.current === section)section.onHidden();});
  register(section);
})();
