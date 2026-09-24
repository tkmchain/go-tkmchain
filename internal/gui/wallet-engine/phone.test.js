import test from 'node:test';
import assert from 'node:assert/strict';
import {ethers} from 'ethers';
import {ml_dsa87} from '@noble/post-quantum/ml-dsa.js';
import {signPhoneDigest,signPhoneDigestWithSeed,phonePublicKey} from './phone.js';
import {createPQKeystore} from './vendor/pq.js';
test('PQ Phone signatures bind the public identity, action and phone domain',async()=>{
 const seed=new Uint8Array(32).fill(7), digest=ethers.toBeHex(1234,32), prefix=ethers.toUtf8Bytes('TKMPHONE_PQ_V1');
 const keyfile=await createPQKeystore('test-only',{seed,scryptN:1024});
 const envelope=ethers.getBytes(await signPhoneDigest(keyfile,'test-only',digest));
 const pub=envelope.slice(prefix.length,prefix.length+2592),sig=envelope.slice(prefix.length+2592);
 assert.equal(sig.length,4627);assert.equal(await phonePublicKey(keyfile,'test-only'),ethers.hexlify(pub));
 assert.ok(ml_dsa87.verify(sig,ethers.getBytes(ethers.concat([prefix,digest])),pub));
 assert.ok(!ml_dsa87.verify(sig,ethers.getBytes(digest),pub));
 await assert.rejects(signPhoneDigest(keyfile,'wrong',digest),/password/);
 assert.throws(()=>signPhoneDigestWithSeed(seed,'0x01'),/32 bytes/);
});
import {derivePhoneEncryptionSeed, encryptPhoneV2, decryptPhoneV2, phoneEncryptionPublicKey} from './phone-crypto.js';
test('Antartical phone envelope round-trips with fixed-size recipient binding', () => {
 const seed = new Uint8Array(32).fill(9), kemSeed = derivePhoneEncryptionSeed(seed), pub = phoneEncryptionPublicKey(kemSeed);
 const from = '+8979000000001', to = '+8979000000002', nonce = '0x' + '22'.repeat(12);
 const envelope = encryptPhoneV2(pub, 8979, from, to, nonce, 'desktop hello');
 assert.equal((envelope.length - 2) / 2, 5785);
 assert.equal(new TextDecoder().decode(decryptPhoneV2(kemSeed, envelope, 8979, from, to, nonce)), 'desktop hello');
 assert.throws(() => decryptPhoneV2(kemSeed, envelope, 8979, to, from, nonce));
});
