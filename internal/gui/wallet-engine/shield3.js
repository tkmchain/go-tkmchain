import {ethers} from 'ethers';
import {decryptPQKeystore} from './vendor/pq.js';
import {normalizeKeystore} from './vendor/keystore.js';
import {acquireWalletOperation} from './operation-lock.js';

export const SHIELD3_MAX_SEND_WEI = 5_000_000n * 10n**18n;
export function validateShield3Amount(value) {
 const amount=ethers.parseUnits(String(value),18);
 if(amount<=0n || amount>SHIELD3_MAX_SEND_WEI) throw Error('Shield3 sends must be greater than zero and at most 5,000,000 TKM.');
 return amount;
}
export function validateShield3Payments(payments) {
 if(!Array.isArray(payments)||payments.length<1||payments.length>3)throw Error('Choose one to three Shield3 recipients.');
 let total=0n;
 const requests=payments.map(payment=>{
  if(!payment.recipient_address?.startsWith('tkmshield3.'))throw Error('Every recipient requires a Shield3 receiving address.');
  const amount=validateShield3Amount(payment.amount);total+=amount;
  if(total>SHIELD3_MAX_SEND_WEI)throw Error('The combined payment amount must not exceed 5,000,000 TKM.');
  return {recipient:payment.recipient_address,amountWei:amount.toString()};
 });
 return {requests,total};
}

export async function shield3Status({rpcURL,rpcToken}) {
 const response=await fetch(rpcURL,{method:'POST',headers:{'Content-Type':'application/json','X-GUI-Token':rpcToken},body:JSON.stringify({jsonrpc:'2.0',id:1,method:'tkmprivacy_shieldedV3Status',params:[]})});
 const result=await response.json();if(result.error){if(result.error.code===-32601)return {active:false};throw Error(result.error.message)};return result.result;
}
export async function validateShield3Recipient(recipient,{walletURL,rpcToken}) {
 return walletRequest('validate',{recipient},{walletURL,rpcToken});
}
async function walletRequest(operation,body,{walletURL,rpcToken}) {
 const base=walletURL||new URL('/shield3',globalThis.location.href).href;
 const target=new URL(base,globalThis.location?.href);
 if(!['127.0.0.1','localhost','[::1]'].includes(target.hostname) || target.origin!==globalThis.location.origin) throw Error('Shield3 private operations require your local wallet.');
 const response=await fetch(target.href.replace(/\/$/,'')+'/'+operation,{method:'POST',headers:{'Content-Type':'application/json','X-GUI-Token':rpcToken},body:JSON.stringify(body)});
 const result=await response.json();if(!response.ok || result.error)throw Error(result.error||'Shield3 wallet operation failed.');return result;
}
async function privateOperation(operation,options,extra={}) {
 const normalized=normalizeKeystore(options.keystore),seed=await decryptPQKeystore(normalized,options.password);
 try{return await walletRequest(operation,{seed:ethers.hexlify(seed),stamp:normalized.shield3Stamp,account:ethers.getAddress('0x'+normalized.address.replace(/^0x/,'')),...extra},options)}finally{seed.fill(0)}
}
export function shield3Identity(options){return privateOperation('identity',options)}
export function shield3ViewKeys(options){return privateOperation('viewkeys',options,{scope:options.scope||'incoming'})}
export function shield3ViewScan(options){return walletRequest('view-scan',{view:options.view},options)}
export function shield3ViewStamp(options){return walletRequest('view-stamp',{stampDisclosure:options.stampDisclosure},options)}
export function shield3FetchRelayOffer(options){return walletRequest('fetch-relay-offer',{relayURL:options.relayURL,requestId:options.requestId},options)}
export function shield3Scan(options){return privateOperation('scan',options)}
export async function sendShield3(options) {
 const release=acquireWalletOperation();
 try{
  const payments=options.intent.payments||[options.intent];
  const {requests}=validateShield3Payments(payments);
  const requestId=options.requestId||ethers.hexlify(ethers.randomBytes(16));
  options.onProgress?.('Building the private Shield3 proof…');
  let result;
  if(options.relayURL){
   const packet=await privateOperation('prepare-relay',options,{payments:requests,relay:options.relayOffer,relayURL:options.relayURL,requestId});
   options.onPrepared?.(packet);
   options.onProgress?.('Submitting the saved payment to the shared relay…');
   for(let attempt=0;attempt<3;attempt++){
    try{result=await shield3Relay({...options,stage:'retry',requestId});if(!result.submissionUncertain||attempt===2)break;}
    catch(error){if(attempt===2)throw error;}
    options.onProgress?.('Relay submission uncertain. Retrying the same saved transaction…');
    await new Promise(resolve=>setTimeout(resolve,2000));
   }
  }else result=await privateOperation('send',options,{payments:requests,requestId});
  options.onSubmitted?.(0,result.transactionHash);if(result.submissionError||result.submissionUncertain) options.onProgress?.('Submission status is uncertain. Check this hash before retrying: '+result.transactionHash);
  return [result.transactionHash];
 }finally{release()}
}
export async function shield3Funds(options) {
 const release=acquireWalletOperation();
 try{return await privateOperation('shield',options,{amountWei:validateShield3Amount(options.amount).toString(),requestId:options.requestId||ethers.hexlify(ethers.randomBytes(16))})}finally{release()}
}

export async function shield3RegisterStamp(options) {
 const release=acquireWalletOperation();
 try{return await privateOperation('register-stamp',options,{requestId:options.requestId})}finally{release()}
}

export async function shield3StampSponsorship(options) {
 const operations={offer:'stamp-offer',authorize:'authorize-stamp',review:'review-sponsorship',submit:'sponsor-stamp'};
 const operation=operations[options.stage];if(!operation)throw Error('Choose a stamp sponsorship step.');
 const release=acquireWalletOperation();
 try{return await privateOperation(operation,options,{recipient:options.recipient,sponsorship:options.sponsorship,requestId:options.requestId})}finally{release()}
}

export async function shield3Relay(options) {
 const operation={offer:'relay-offer',prepare:'prepare-relay',review:'review-relay',submit:'submit-relay',status:'relay-status',retry:'submit-relay-draft'}[options.stage];
 if(!operation)throw Error('Choose a relay step.');
 if(options.stage==='status'||options.stage==='retry')return walletRequest(operation,{account:options.account||ethers.getAddress('0x'+normalizeKeystore(options.keystore).address.replace(/^0x/,'')),requestId:options.requestId},options);
 const release=acquireWalletOperation();
 try{return await privateOperation(operation,options,{relay:options.relay,relayTransaction:options.transaction,recipient:options.recipient,amountWei:options.amount?validateShield3Amount(options.amount).toString():undefined,requestId:options.requestId})}finally{release()}
}
export function shield3ReviewRelayOffer(options){return walletRequest('review-relay-offer',{relay:options.relay},options)}
export async function shield3Disclosure(options){
 if(options.stage==='verify')return walletRequest('verify-disclosure',{disclosure:options.disclosure,capsule:options.capsule,auditKey:options.auditKey},options);
 return privateOperation(options.stage==='key'?'disclosure-key':'export-disclosure',options,{transactionHash:options.transactionHash,outputIndex:options.outputIndex,auditPublicKey:options.auditPublicKey});
}
