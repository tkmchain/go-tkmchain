import {ethers} from 'ethers';
import {normalizeKeystore} from './vendor/keystore.js';
import {decryptPQKeystore,signPQTkmTransactionWithSeed} from './vendor/pq.js';
import {TkmChainAPI} from './vendor/api.js';
import {ShieldedProverClient,deriveShieldedIdentity,clearShieldedIdentity,scanShieldedNotes,validateProverTransaction,validateV2Withdrawal,validateV2GasSponsor,shieldedMimcHash} from './vendor/shielded.js';
import {acquireWalletOperation} from './operation-lock.js';
const GAS=3000000n;
export function validateShield2Migration(response,{address,note,nonce,gasPrice}){
 const tx=response.transaction;
 validateProverTransaction(tx,{chainId:8979,value:0n});
 validateV2Withdrawal(tx,response,{recipient:address,valueWei:BigInt(note.noteValueWei)});
 if(BigInt(tx.nonce)!==BigInt(nonce)||BigInt(tx.gas)!==GAS||BigInt(tx.gasFeeCap)!==gasPrice||BigInt(tx.gasTipCap)!==gasPrice||tx.accessList?.length||validateV2GasSponsor(tx,response)!==0n)throw Error('Migration proof changed nonce, fees or gas sponsorship.');
 const envelope=ethers.decodeRlp(ethers.getBytes(tx.data).slice(10));
 if(envelope[1]?.length!==1||ethers.hexlify(envelope[1][0][0]).toLowerCase()!==note.nullifier.toLowerCase()||ethers.hexlify(envelope[1][0][1]).toLowerCase()!==note.merkleRoot.toLowerCase()||envelope[8]?.length!==4)throw Error('Migration proof changed the input or omitted empty-output openings.');
 for(let slot=0;slot<4;slot++){
  const random=BigInt(envelope[8][slot]);
  if(random<=0n||random>=21888242871839275222246405745257275088548364400416034343698204186575808495617n)throw Error('Migration proof returned invalid randomness.');
  const commitment=ethers.toBeHex(shieldedMimcHash([2001n,slot===0?BigInt(address):0n,1n,0n,random]),32);
  if(commitment.toLowerCase()!==ethers.hexlify(envelope[2][slot][0]).toLowerCase())throw Error('Migration proof creates private change.');
 }
 return tx;
}
// Withdraw one entire legacy note to its own PQ account. Wait for its receipt
// before migrating another note or shielding the confirmed public balance.
export async function migrateShield2Note(options){
 const release=acquireWalletOperation();let seed,identity,provider;
 try{
  const normalized=normalizeKeystore(options.keystore);seed=await decryptPQKeystore(normalized,options.password);
  const address=ethers.getAddress('0x'+normalized.address.replace(/^0x/,''));identity=deriveShieldedIdentity(seed,address,8979);
  const connection=new ethers.FetchRequest(options.rpcURL);connection.setHeader('X-GUI-Token',options.rpcToken);
  provider=new ethers.JsonRpcProvider(connection);
  if(BigInt(await provider.send('eth_chainId',[]))!==8979n||!(await provider.send('tkmprivacy_shieldedV3Status',[])).active)throw Error('Migration requires Antartical on TKM mainnet.');
  const api=new TkmChainAPI(provider),state={lastScannedBlock:-1,notes:[]};
  options.onProgress?.('Scanning legacy notes…');await scanShieldedNotes(api,identity,state,()=>{});
  const chosen=state.notes.find(n=>n.status==='available'&&Number(n.version)===2&&BigInt(n.assetId)===1n);
  if(!chosen)throw Error('No confirmed Shield2 note remains to migrate.');
  const nonce=await api.getTransactionCount(address,'latest');if(BigInt(await api.getTransactionCount(address,'pending'))!==BigInt(nonce))throw Error('Wait for your pending transaction to confirm.');
  const gasPrice=BigInt(await provider.send('eth_gasPrice',[])),balance=BigInt(await provider.send('eth_getBalance',[address,'latest']));
  if(balance+BigInt(chosen.noteValueWei)<GAS*gasPrice)throw Error('This note and public balance cannot cover migration gas.');
  if((await api.privacyNullifierStatus(chosen.nullifier))?.spent)throw Error('Legacy note is already spent. Rescan.');
  const path=await api.privacyCommitmentPath(chosen.commitment);if(!path.found)throw Error('Legacy note is no longer confirmed.');
  const note={...chosen,merkleRoot:path.root,merklePath:path.merklePath.map(String),merklePathIndex:path.merklePathIndex.map(v=>BigInt(v).toString())};
  const proverURL=new URL('/prover',globalThis.location.href);
  const fetcher=(url,request)=>fetch(url,{...request,headers:{...request.headers,'X-GUI-Token':options.rpcToken}});
  const prover=new ShieldedProverClient(proverURL.href,'',fetcher);
  options.onProgress?.('Building the full-note migration proof…');
  const response=await prover.buildWithdrawal({requestId:ethers.hexlify(ethers.randomBytes(16)),from:address,to:address,amountWei:note.noteValueWei,note,changeViewKey:ethers.hexlify(identity.viewPublicKey),nonce,gasPriceWei:gasPrice.toString()});
  const tx=validateShield2Migration(response,{address,note,nonce,gasPrice});
  const signed=signPQTkmTransactionWithSeed(tx,seed),hash=ethers.keccak256(signed.rawTransaction);
  options.onSubmitted?.(hash,ethers.formatEther(BigInt(note.noteValueWei)));
  try{await api.sendRawTransaction(signed.rawTransaction)}catch(error){throw Error('Migration submission status uncertain: '+hash+'. Check Activity before retrying. '+error.message)}
  return hash;
 }finally{seed?.fill(0);clearShieldedIdentity(identity);provider?.destroy();release()}
}
