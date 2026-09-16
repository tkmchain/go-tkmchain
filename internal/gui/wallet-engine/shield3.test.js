import test from 'node:test';
import assert from 'node:assert/strict';
import {validateShield3Amount, SHIELD3_MAX_SEND_WEI} from './shield3.js';
test('Shield3 amount cap uses exact smallest units',()=>{
 assert.equal(validateShield3Amount('5000000'),SHIELD3_MAX_SEND_WEI);
 assert.equal(validateShield3Amount('4999999.999999999999999999'),SHIELD3_MAX_SEND_WEI-1n);
 assert.equal(validateShield3Amount('0.000000000000000001'),1n);
 for(const amount of ['0','-1','5000000.000000000000000001','1e6','NaN','0.0000000000000000001'])assert.throws(()=>validateShield3Amount(amount));
});

import {validateShield3Payments} from './shield3.js';
test('batch limit applies to combined payment amount and preserves exact units',()=>{
 const payment=amount=>({recipient_address:'tkmshield3.test',amount});
 const {requests,total}=validateShield3Payments([payment('2500000'),payment('2499999.999999999999999999'),payment('0.000000000000000001')]);
 assert.equal(total,SHIELD3_MAX_SEND_WEI);assert.equal(requests.length,3);assert.equal(requests[2].amountWei,'1');
 for(const payments of [[],[payment('2500000'),payment('2500000.000000000000000001')],[payment('1'),payment('1'),payment('1'),payment('1')],[{recipient_address:'0x1234',amount:'1'}]])assert.throws(()=>validateShield3Payments(payments));
});

import {sendShield3,shield3Relay} from './shield3.js';
import {createPQKeystore} from './vendor/pq.js';
test('automatic relay retries only the prepared request and polls without a seed',async()=>{
 const originalFetch=globalThis.fetch,originalLocation=globalThis.location;
 globalThis.location={href:'http://127.0.0.1:8080/',origin:'http://127.0.0.1:8080'};
 const keystore=await createPQKeystore('test-password',{seed:new Uint8Array(32),scryptN:1024});
 const calls=[],hash='0x'+'4'.repeat(64);let attempt=0;
 globalThis.fetch=async(url,options)=>{
  const body=JSON.parse(options.body),operation=new URL(url).pathname.split('/').pop();calls.push({operation,body});
  if(operation==='prepare-relay')return{ok:true,json:async()=>({transaction:'0x1234',requestId:body.requestId})};
  if(operation==='submit-relay-draft'){attempt++;if(attempt===1)throw Error('lost HTTP reply');return{ok:true,json:async()=>({transactionHash:hash,status:'unconfirmed'})};}
  return{ok:true,json:async()=>({transactionHash:hash,status:'confirmed'})};
 };
 try{
  const requestId='stable-relay-request-id',options={keystore,password:'test-password',rpcToken:'private-token',relayURL:'https://relay.example',relayOffer:{version:1},requestId,intent:{payments:[{recipient_address:'tkmshield3.first',amount:'1'},{recipient_address:'tkmshield3.second',amount:'2'}]}};
  assert.deepEqual(await sendShield3(options),[hash]);
  assert.equal(calls.filter(call=>call.operation==='prepare-relay').length,1);
  const retries=calls.filter(call=>call.operation==='submit-relay-draft');assert.equal(retries.length,2);
  for(const {body} of retries){assert.equal(body.requestId,requestId);assert.equal(body.seed,undefined);assert.equal(body.relayTransaction,undefined);}
  const status=await shield3Relay({account:'0x'+keystore.address,rpcToken:'private-token',stage:'status',requestId});assert.equal(status.status,'confirmed');assert.equal(calls.at(-1).body.seed,undefined);
 }finally{globalThis.fetch=originalFetch;globalThis.location=originalLocation;}
});
