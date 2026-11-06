import test from 'node:test';
import assert from 'node:assert/strict';
import {shield6Operation,shield6RequestStatus} from './shield3.js';
import {createPQKeystore} from './vendor/pq.js';

test('Shield6 rejects invalid versions, missing stable IDs and non-stamped recipients before unlocking',async()=>{
 for(const options of [{operation:'unsupported'},{operation:'send'},{operation:'send',requestId:'stable-id',amount:'1',recipient:'tkmshield6.old'},{operation:'shield',requestId:'stable-id',amount:'0'},{operation:'shield',requestId:'stable-id',amount:'340282366920938463464'}])await assert.rejects(shield6Operation(options));
});

test('Shield6 retries preserve request identity and status requests contain no seed',async()=>{
 const oldFetch=globalThis.fetch,oldLocation=globalThis.location;
 globalThis.location={href:'http://127.0.0.1:8080/',origin:'http://127.0.0.1:8080'};
 const keystore=await createPQKeystore('test-password',{seed:new Uint8Array(32),scryptN:1024});
 const calls=[];let first=true;
 globalThis.fetch=async(url,options)=>{
  const body=JSON.parse(options.body);calls.push({url,body});
  if(first){first=false;throw Error('lost HTTP reply');}
  return {ok:true,json:async()=>({transactionHash:'0x'+'1'.repeat(64)})};
 };
 try {
  const options={operation:'send',keystore,password:'test-password',rpcToken:'private-token',requestId:'stable-shield6-request',recipient:'tkmshield6r.test-code',amount:'0.000000000000000001'};
  await assert.rejects(shield6Operation(options),/lost HTTP reply/);
  await shield6Operation(options);
  assert.deepEqual(calls[0].body,calls[1].body);
  assert.equal(calls[1].body.proofVersion,6);
  assert.equal(calls[1].body.amountWei,'1');
  assert.equal(new URL(calls[1].url).origin,'http://127.0.0.1:8080');
  await shield6RequestStatus({account:'0x'+keystore.address,requestId:options.requestId,rpcToken:'private-token'});
  assert.equal(calls.at(-1).body.seed,undefined);
  assert.equal(calls.at(-1).body.proofVersion,6);
 } finally { globalThis.fetch=oldFetch;globalThis.location=oldLocation; }
});
