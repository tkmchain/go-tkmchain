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
export function shield3ViewKeys(options){return privateOperation('viewkeys',options)}
export function shield3Scan(options){return privateOperation('scan',options)}
export async function sendShield3(options) {
 const release=acquireWalletOperation();
 try{
  const amount=validateShield3Amount(options.intent.amount);
  if(!options.intent.recipient_address?.startsWith('tkmshield3.'))throw Error('Antartical sends require a Shield3 receiving address.');
  options.onProgress?.('Building the private Shield3 proof…');
  const result=await privateOperation('send',options,{recipient:options.intent.recipient_address,amountWei:amount.toString(),requestId:options.requestId||ethers.hexlify(ethers.randomBytes(16))});
  options.onSubmitted?.(0,result.transactionHash);if(result.submissionError) options.onProgress?.('Submission status is uncertain. Check this hash before retrying: '+result.transactionHash);
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
