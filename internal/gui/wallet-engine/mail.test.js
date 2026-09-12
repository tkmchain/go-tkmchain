import test from 'node:test';
import assert from 'node:assert/strict';
import {ethers} from 'ethers';
import {x25519} from '@noble/curves/ed25519';
import {xchacha20poly1305} from '@noble/ciphers/chacha.js';
import {hkdf} from '@noble/hashes/hkdf';
import {sha256} from '@noble/hashes/sha256';
import {ml_dsa87} from '@noble/post-quantum/ml-dsa.js';
import {runMailOperation,assertMailPlan,planMailFunding} from './mail.js';
import {createPQKeystore} from './vendor/pq.js';
import {deriveShieldedIdentity,shieldedMimcHash} from './vendor/shielded.js';
import {deriveEmailIdentity,decryptEmailMessage} from './vendor/email-crypto.js';
import {emailRegistryHash} from './vendor/email-registry.js';
const encode=action=>ethers.hexlify(ethers.toUtf8Bytes('TKMEMAILVM1'+JSON.stringify({v:3,...action})));
const emptyOutput=()=>[ethers.ZeroHash,ethers.ZeroHash,'0x','0x','0x','0x'];
async function fixture({funding=false,tamper=false,loseResponse=false}={}) {
 const seed=new Uint8Array(32).fill(7),peerSeed=new Uint8Array(32).fill(8),password='test-only-password';
 const keystore=await createPQKeystore(password,{seed,scryptN:1024});
 const address=ethers.getAddress('0x'+keystore.address),identity=deriveShieldedIdentity(seed,address),peerEmail=deriveEmailIdentity(peerSeed);
 const recipient='0x'+'1'.repeat(40),outputs=[],spent=new Set(),hashes=[],traffic=[],reviews=[];
 let serial=0,nonce=0,pending=null,key='',registered=false,sent=null;
 const note=(value)=>{
  const randomness=BigInt(++serial),commitment=ethers.toBeHex(shieldedMimcHash([2001n,BigInt(address),1n,BigInt(value),randomness]),32);
  const opening={format:'TKM_SHIELDED_NOTE_PAYLOAD_V4',version:2,recipient:address,assetId:'1',noteValueWei:String(value),noteRandomness:String(randomness),commitment,nullifier:ethers.toBeHex(serial,32)};
  const ephemeral=new Uint8Array(32).fill(9),shared=x25519.getSharedSecret(ephemeral,identity.viewPublicKey),info=ethers.toUtf8Bytes('TKM_SHIELDED_NOTE_X25519_XCHACHA20POLY1305_V1');
  const material=hkdf(sha256,shared,ethers.getBytes(commitment),info,33),iv=new Uint8Array(24).fill(serial);
  const payload=xchacha20poly1305(material.slice(0,32),iv,ethers.getBytes(ethers.concat([info,commitment]))).encrypt(ethers.toUtf8Bytes(JSON.stringify(opening)));
  return {opening,output:{commitment,payloadHash:ethers.keccak256(payload),ephemeralPubKey:ethers.hexlify(x25519.getPublicKey(ephemeral)),viewTag:ethers.hexlify(material.slice(32)),encryptedPayload:ethers.hexlify(payload),nonce:ethers.hexlify(iv),transactionHash:ethers.toBeHex(serial,32),outputIndex:'0x0',blockNumber:'0x1'}};
 };
 if(!funding)outputs.push(note(100n).output);
 const rpc=async(method,params)=>{
  traffic.push(JSON.stringify([method,params]));
  const values={eth_chainId:'0x2313',tkmprivacy_shieldedV2Active:true,tkmprivacy_shieldedGasSponsorActive:true,eth_gasPrice:'0x1',eth_getBalance:'0x989680',eth_blockNumber:'0x1',tkmprivacy_commitmentActivationTime:'0x0',eth_getBlockByNumber:{timestamp:'0x1'},tkmdomain_subscriberUnitPrice:'0x28',tkmdomain_pending:[]};
  if(method in values)return values[method];
  if(method==='eth_getTransactionCount')return ethers.toQuantity(nonce);
  if(method==='tkmdomain_domain')return {operator:recipient,payoutAddress:recipient};
  if(method==='tkmdomain_buy')return {applicationData:encode({kind:'buy',username:params[0],domain:params[1],registryHash:emailRegistryHash('mailbox',params[0]+'@'+params[1])}),registryHash:emailRegistryHash('mailbox',params[0]+'@'+params[1]),withdrawalRecipient:recipient,totalWithdrawalAmountWei:'0x28'};
  if(method==='tkmdomain_mailbox'){assert.ok(registered);return {owner:address,encryptionKey:key};}
  if(method==='emailvm_publishKey')return {applicationData:encode({kind:'key',mailbox:params[0],key:params[1].slice(2)})};
  if(method==='emailvm_key')return {publicKey:params[0]==='bob@tkm'?ethers.hexlify(peerEmail.publicKey):key};
  if(method==='emailvm_send')return {applicationData:encode({kind:'message',from:params[0],to:params[1],ciphertext:params[2].slice(2),nonce:params[3].slice(2)})};
  if(method==='tkmprivacy_shieldedOutputs')return outputs;
  if(method==='tkmprivacy_commitmentPath')return {found:true,root:ethers.ZeroHash,merklePath:[],merklePathIndex:[]};
  if(method==='tkmprivacy_nullifierStatus')return {spent:spent.has(params[0]),spentHeight:'0x1'};
  if(method==='eth_sendRawTransaction'){
   const raw=params[0],fields=ethers.decodeRlp(ethers.getBytes(raw).slice(1));assert.ok(raw.startsWith('0x06'));
   const digest=ethers.keccak256(ethers.concat(['0x06',ethers.encodeRlp(fields.slice(0,-1))]));
   assert.ok(ml_dsa87.verify(ethers.getBytes(fields.at(-1)),ethers.getBytes(digest),ethers.getBytes(fields[10])));
   assert.equal(fields[7],pending.transaction.data);nonce++;
   if(pending.note)spent.add(pending.note.nullifier);
   if(pending.created)outputs.push(pending.created.output);
   if(pending.action?.kind==='buy')registered=true;
   if(pending.action?.kind==='key')key='0x'+pending.action.key;
   if(pending.action?.kind==='message')sent=pending.action;
   const hash=ethers.keccak256(raw);hashes.push(hash);
   if(loseResponse)throw Error('connection lost');return hash;
  }
  if(method==='eth_getTransactionReceipt')return {status:'0x1'};
  throw Error('Unexpected RPC '+method);
 };
 const proverFetch=async(url,options)=>{
  if(url.endsWith('healthz'))return {ok:true,json:async()=>({ok:true,hasProvingKeyV2:true,withdrawalBuildReady:true})};
  traffic.push(options.body);const req=JSON.parse(options.body),deposit=url.endsWith('build-deposit'),amount=BigInt(req.amountWei);
  const change=deposit?amount:BigInt(req.note.noteValueWei)-amount;
  const created=change>0n?note(change):null;
  const action=req.applicationData?JSON.parse(ethers.toUtf8String(ethers.getBytes(req.applicationData).slice(11))):null;
  const slots=Array.from({length:4},emptyOutput);if(created)slots[0][0]=created.opening.commitment;
  const spend=deposit?[]:[[req.note.nullifier,ethers.ZeroHash,'0x1234',tamper?'0x1234':req.applicationData]];
  const envelope=['0x02',spend,slots,ethers.ZeroHash,ethers.ZeroHash,deposit?ethers.ZeroAddress:req.to,deposit?'0x':ethers.toBeHex(amount),'0x'];
  const transaction={chainId:'0x2313',nonce:req.nonce,gasTipCap:'0x1',gasFeeCap:'0x1',gas:'0x2dc6c0',to:'0x00000000000000000000000000000000000000f7',value:deposit?ethers.toQuantity(amount):'0x0',data:ethers.concat([ethers.toUtf8Bytes('TKMSHIELD1'),ethers.encodeRlp(envelope)]),accessList:[]};
  pending={transaction,note:req.note,created,action};
  return {ok:true,json:async()=>({transaction,shieldedVersion:2,gasSponsorWei:'0',spentNullifier:req.note?.nullifier,outputOpenings:created?[{index:0,recipient:address,assetId:'1',valueWei:String(change),randomness:created.opening.noteRandomness,commitment:created.opening.commitment}]:[]})};
 };
 const recorded=[];
 const options={keystore,password,rpc,proverURL:'http://127.0.0.1/prover',proverFetch,onReview:async review=>{reviews.push(review);return true;},onSubmitted:hash=>recorded.push(hash)};
 return {options,hashes,recorded,traffic,reviews,peerEmail,getSent:()=>sent};
}
test('Mail registers, publishes a recoverable key, encrypts, signs and confirms',async()=>{
 const f=await fixture();
 await runMailOperation({...f.options,operation:'buy',params:{username:'alice',domain:'tkm'}});
 assert.equal(f.hashes.length,2);assert.equal(f.reviews.length,2);
 await runMailOperation({...f.options,operation:'send',params:{from:'alice@tkm',to:'bob@tkm',subject:'Hello',body:'Unicode: 🌍'}});
 assert.equal(f.hashes.length,3);assert.deepEqual(f.hashes,f.recorded);
 const message=f.getSent(),own=deriveEmailIdentity(new Uint8Array(32).fill(7));
 assert.equal(decryptEmailMessage(f.peerEmail.privateKey,own.publicKey,message.from,message.to,ethers.getBytes('0x'+message.ciphertext),ethers.getBytes('0x'+message.nonce)),'Hello\n\nUnicode: 🌍');
 assert.ok(f.traffic.every(t=>!t.includes('Unicode:')&&!t.includes(f.options.password)&&!t.includes(f.options.keystore.crypto.ciphertext)));
 const plaintext=await runMailOperation({...f.options,operation:'decrypt',params:{mailbox:'alice@tkm',message:{...message,ciphertext:'0x'+message.ciphertext,nonce:'0x'+message.nonce}}});
 assert.equal(plaintext,'Hello\n\nUnicode: 🌍');
});
test('Mail self-shields public funds before paying the registration',async()=>{
 const f=await fixture({funding:true});await runMailOperation({...f.options,operation:'buy',params:{username:'alice',domain:'tkm'}});
 assert.equal(f.hashes.length,4);
});
test('Mail refuses changed proof-bound actions without broadcasting',async()=>{
 const f=await fixture({tamper:true});await assert.rejects(runMailOperation({...f.options,operation:'buy',params:{username:'alice',domain:'tkm'}}),/changed or omitted/);assert.equal(f.hashes.length,0);
});
test('Mail preserves attempted hash on uncertain broadcast and does not retry',async()=>{
 const f=await fixture({loseResponse:true});await assert.rejects(runMailOperation({...f.options,operation:'buy',params:{username:'alice',domain:'tkm'}}),/Attempted transaction hashes/);assert.equal(f.hashes.length,1);assert.deepEqual(f.hashes,f.recorded);
});
test('Mail cancellation sends nothing and action validation rejects a changed recipient',async()=>{
 const f=await fixture();await assert.rejects(runMailOperation({...f.options,onReview:async()=>false,operation:'buy',params:{username:'alice',domain:'tkm'}}),/cancelled/);assert.equal(f.hashes.length,0);
 assert.throws(()=>assertMailPlan({applicationData:encode({kind:'message',to:'attacker@tkm'})},{kind:'message',to:'bob@tkm'}),/changed/);
});

test('Mail funding checks the complete gas budget before partial payments',()=>{
 const note={status:'available',version:2,assetId:'1',noteValueWei:'100'};
 assert.deepEqual(planMailFunding([note],90n,10n,0n,true),{transactions:1,maxGas:10n});
 assert.throws(()=>planMailFunding([note],100n,10n,0n,true),/Insufficient total/);
 assert.deepEqual(planMailFunding([],40n,10n,60n,true),{transactions:2,maxGas:20n});
 assert.throws(()=>planMailFunding([],40n,10n,59n,true),/Insufficient total/);
});
