(function () {
 'use strict';
 const {rpc,el,fmtTKM}=window.GUI;
 async function form(container,title) {
  container.body.replaceChildren();
  const account=el('select',{class:'txt','aria-label':'Wallet account'});
  for(const value of await rpc('eth_accounts',[]))account.appendChild(el('option',{value,text:value}));
  const password=el('input',{class:'txt',type:'password',autocomplete:'current-password','aria-label':'Wallet password'});
  const status=el('p',{role:'status',class:'wallet-status'});
  container.body.append(el('h3',{text:title}),el('label',{text:'Wallet account'}),account,el('label',{text:'Wallet password'}),password,status);
  return {account,password,status};
 }
 function requestKey(account,operation) {return 'tkm-shield6-request-v2:'+account+':'+operation;}
 function stableRequest(account,operation,fields) {
  const key=requestKey(account,operation),previous=JSON.parse(localStorage.getItem(key)||'null');
  const encoded=JSON.stringify(fields);
  if(previous&&previous.fields!==encoded)throw Error('An earlier Shield6 request is pending. Retry it or confirm its transaction before changing payment fields.');
  const record=previous||{id:crypto.randomUUID(),fields:encoded};localStorage.setItem(key,JSON.stringify(record));return record;
 }
 async function run(unlock,fields,operation,extra={}) {
  const {engine,options}=await unlock(fields.account.value,fields.password.value);
  return engine.shield6Operation({...options,operation,...extra});
 }
 GUI.renderShield6Send=async function(container,unlock) {
  const f=await form(container,'Shield6 private payment');
  const recipient=el('textarea',{class:'txt',rows:'4',spellcheck:'false','aria-label':'Recipient payment code',placeholder:'tkmshield6r.…'});
  const amount=el('input',{class:'txt',inputmode:'decimal','aria-label':'Amount in TKM'});
  const mode=el('select',{class:'txt','aria-label':'Payment type'},[el('option',{value:'send',text:'Private payment'}),el('option',{value:'withdraw',text:'Public withdrawal to stamped address'})]);
  const send=el('button',{class:'btn gold',text:'Send private payment'}),check=el('button',{class:'btn secondary',text:'Check pending transaction'});
  container.body.append(mode,el('p',{class:'dim',text:'Public withdrawals expose the destination and amount. Private payments use a recipient-stamped payment code.'}),el('label',{text:'Recipient payment code or stamped public address'}),recipient,el('label',{text:'Amount in TKM'}),amount,send,check);
  send.onclick=async()=>{
   send.disabled=check.disabled=true;
   try {
    const fields={recipient:recipient.value.trim(),amount:amount.value.trim(),mode:mode.value},record=stableRequest(f.account.value,'send',fields);
    f.status.textContent='Constructing private payment and change proofs. Keep this wallet open.';
    const result=await run(unlock,f,mode.value,{...fields,requestId:record.id});
    record.hash=result.transactionHash;localStorage.setItem(requestKey(f.account.value,'send'),JSON.stringify(record));
    f.status.textContent=(result.submissionError?'Submission uncertain. Retry this same payment: ':'Payment submitted: ')+result.transactionHash;
   } catch(error){f.status.textContent=error.message;}finally{f.password.value='';send.disabled=check.disabled=false;}
  };
  check.onclick=async()=>{
   try {
    const key=requestKey(f.account.value,'send'),record=JSON.parse(localStorage.getItem(key)||'null');
    if(!record){f.status.textContent='No pending request.';return;}
    const result=await (await GUI.engine()).shield6RequestStatus({account:f.account.value,requestId:record.id,rpcToken:GUI.rpcToken()});
    if(!result.found){localStorage.removeItem(key);f.status.textContent='No signed transaction was saved. You can edit the payment.';return;}
    record.hash=result.transactionHash;localStorage.setItem(key,JSON.stringify(record));
    f.status.textContent=(result.included?(result.successful===false?'Transaction included but failed: ':'Transaction included: '):result.expired?'Request expired; a fresh request can be created: ':'Transaction remains unconfirmed: ')+record.hash;
    if(result.included||result.obsolete||result.expired)localStorage.removeItem(key);
   } catch(error){f.status.textContent=error.message;}
  };
 };
 GUI.renderShield6Receive=async function(container,unlock,stampGate) {
  const f=await form(container,'Shield6 receive and deposit');
  const code=el('textarea',{class:'txt',rows:'5',readOnly:true,'aria-label':'Recipient-stamped payment code'});
  const check=el('button',{class:'btn secondary',text:'Check registration / deposit'});
  const receive=el('button',{class:'btn',text:'Show payment code'}),register=el('button',{class:'btn secondary',text:'Register recipient keys'}),scan=el('button',{class:'btn secondary',text:'Refresh private balance'});
  const amount=el('input',{class:'txt',inputmode:'decimal','aria-label':'Deposit amount in TKM'}),deposit=el('button',{class:'btn gold',text:'Deposit public TKM'});
  container.body.append(el('p',{class:'dim',text:'Register the account stamp first. Recipient-key registration is permanent and pays gas. Share the payment code privately. Deposits expose the account and deposit amount.'}),receive,register,scan,check,code,el('label',{text:'Public deposit amount in TKM'}),amount,deposit);
  const updateStamp=stampGate(container,f.account,f.password,[receive,register,deposit]);
  await updateStamp();
  check.onclick=async()=>{
   try {
    for(const operation of ['register-stamp','shield']) {
     const key=requestKey(f.account.value,operation),record=JSON.parse(localStorage.getItem(key)||'null');
     if(!record)continue;
     const result=await (await GUI.engine()).shield6RequestStatus({account:f.account.value,requestId:record.id,rpcToken:GUI.rpcToken()});
     if(!result.found||result.included||result.obsolete||result.expired)localStorage.removeItem(key);
     f.status.textContent=result.included?(result.successful===false?'Transaction included but failed: ':'Transaction included: ')+result.transactionHash:result.expired?'Request expired; you can create a fresh request.':result.found?'Transaction remains unconfirmed: '+result.transactionHash:'No saved transaction. You can edit the request.';
    }
   } catch(error){f.status.textContent=error.message;}
  };
  for(const [button,operation] of [[receive,'identity'],[register,'register-stamp'],[scan,'scan'],[deposit,'shield']])button.onclick=async()=>{
   [receive,register,scan,deposit].forEach(b=>b.disabled=true);
   try {
    let extra={};
    if(operation==='register-stamp'||operation==='shield') {
     const fields={amount:operation==='shield'?amount.value.trim():''},record=stableRequest(f.account.value,operation,fields);
     extra={...fields,requestId:record.id};
    }
    f.status.textContent=operation==='shield'?'Constructing deposit proof…':'Working…';
    const result=await run(unlock,f,operation,extra);
    if(result.paymentCode){code.value=result.paymentCode;f.status.textContent='Payment code ready.';}
    else if(result.balanceWei!==undefined)f.status.textContent=fmtTKM(result.balanceWei,18)+' TKM in '+result.noteCount+' spendable notes';
    else {f.status.textContent='Transaction '+(result.status||'submitted')+': '+result.transactionHash;const key=requestKey(f.account.value,operation),record=JSON.parse(localStorage.getItem(key));record.hash=result.transactionHash;localStorage.setItem(key,JSON.stringify(record));}
   } catch(error){f.status.textContent=error.message;}finally{f.password.value='';[receive,register,scan,deposit].forEach(b=>b.disabled=false);await updateStamp();}
  };
 };
})();
