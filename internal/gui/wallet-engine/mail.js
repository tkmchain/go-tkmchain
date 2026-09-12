import {ethers} from 'ethers';
import {decryptPQKeystore, signPQTkmTransactionWithSeed} from './vendor/pq.js';
import {normalizeKeystore} from './vendor/keystore.js';
import {deriveEmailIdentity, clearEmailIdentity, encryptEmailMessage, decryptEmailMessage} from './vendor/email-crypto.js';
import {emailRegistryHash} from './vendor/email-registry.js';
import {TkmChainAPI} from './vendor/api.js';
import {ShieldedProverClient, deriveShieldedIdentity, clearShieldedIdentity, scanShieldedNotes, validateProverTransaction, validateV2GasSponsor, validateV2Withdrawal, validateShieldedApplicationData} from './vendor/shielded.js';
import {validateResponse} from './shield2-send.js';
import {acquireWalletOperation} from './operation-lock.js';
const GAS = 3000000n, MAX_NOTE = (1n << 64n) - 1n;
const lower = value => String(value || '').toLowerCase();
const canonical = value => String(value || '').trim().toLowerCase();

export function planMailFunding(notes, amount, gasCost, publicBalance, sponsored) {
    let remaining=BigInt(amount), balance=BigInt(publicBalance), transactions=0;
    for (const note of notes) {
        if (!remaining) break;
        if (note.status!=='available'||Number(note.version)!==2||BigInt(note.assetId)!==1n) continue;
        const sponsor=balance<gasCost ? gasCost-balance : 0n;
        if (sponsor && !sponsored) break;
        const capacity=BigInt(note.noteValueWei)-sponsor;
        if (capacity<=0n) continue;
        remaining-=remaining<capacity?remaining:capacity; balance-=gasCost-sponsor; transactions++;
    }
    while (remaining>0n) {
        const part=remaining<MAX_NOTE?remaining:MAX_NOTE;
        if (balance<part+2n*gasCost) throw Error('Insufficient total funds for the complete Mail payment and network gas.');
        balance-=part+2n*gasCost; remaining-=part; transactions+=2;
    }
    return {transactions,maxGas:BigInt(transactions)*gasCost};
}

export function assertMailPlan(plan, expected) {
    const raw = ethers.getBytes(plan.applicationData), magic = ethers.toUtf8Bytes('TKMEMAILVM1');
    if (raw.length > 12288 || ethers.hexlify(raw.slice(0,magic.length)) !== ethers.hexlify(magic)) throw Error('Invalid mail action encoding.');
    const action = JSON.parse(ethers.toUtf8String(raw.slice(magic.length)));
    if (action.v !== 3) throw Error('Unsupported mail action version.');
    for (const [key,value] of Object.entries(expected)) if (lower(action[key]) !== lower(value)) throw Error('Node changed the mail action: ' + key);
    return action;
}
export function validateMailWithdrawal(response, {nonce, gasPrice, note, amount, sponsor, recipient, address, applicationData}) {
    const tx = response.transaction;
    validateProverTransaction(tx,{chainId:8979,value:0n});
    if (BigInt(tx.nonce) !== BigInt(nonce) || BigInt(tx.gas) !== GAS || BigInt(tx.gasFeeCap) !== gasPrice || BigInt(tx.gasTipCap) !== gasPrice || !Array.isArray(tx.accessList) || tx.accessList.length) throw Error('Proof builder changed mail transaction fees or nonce.');
    if (validateV2GasSponsor(tx,response) !== sponsor) throw Error('Proof builder changed mail gas sponsorship.');
    const change = BigInt(note.noteValueWei)-amount-sponsor;
    if (change < 0n) throw Error('Insufficient note value for mail payment and gas.');
    validateV2Withdrawal(tx,response,{recipient,valueWei:amount,expectedOutputs:change ? [{index:0,recipient:address,assetId:1n,valueWei:change}] : []});
    validateShieldedApplicationData(tx,applicationData);
    const envelope = ethers.decodeRlp(ethers.getBytes(tx.data).slice(10));
    if (envelope[1].length !== 1 || lower(envelope[1][0][0]) !== lower(note.nullifier) || lower(response.spentNullifier) !== lower(note.nullifier)) throw Error('Proof builder changed the mail input note.');
    return tx;
}

// Each payment is proof-bound, checked locally, signed locally and confirmed.
// A public balance can first fund a self-owned note; it never pays the operator directly.
export async function runMailOperation({keystore,password,rpc,proverURL,proverFetch,operation,params={},onReview,onProgress=()=>{},onSubmitted=()=>{}}) {
    const release = acquireWalletOperation();
    let seed,identity,email;
    const hashes=[];
    try {
        const normalized=normalizeKeystore(keystore);
        seed=await decryptPQKeystore(normalized,password);
        const address=ethers.getAddress('0x'+normalized.address.replace(/^0x/,''));
        identity=deriveShieldedIdentity(seed,address,8979); email=deriveEmailIdentity(seed);
        if (BigInt(await rpc('eth_chainId',[]))!==8979n) throw Error('Mail requires TKM chain 8979.');
        const api=new TkmChainAPI({send:rpc});
        const prover=new ShieldedProverClient(proverURL,'',proverFetch);
        const state={lastScannedBlock:-1,notes:[]};
        const ownKey=ethers.hexlify(email.publicKey);
        const ensureOwner=async mailbox=>{
            const record=await rpc('tkmdomain_mailbox',[mailbox]);
            if (lower(record.owner)!==lower(address)) throw Error('Selected PQ account does not own '+mailbox);
            return record;
        };
        const broadcast=async tx=>{
            const signed=signPQTkmTransactionWithSeed(tx,seed), hash=ethers.keccak256(signed.rawTransaction);
            hashes.push(hash); await onSubmitted(hash); // persist before an uncertain RPC response
            const returned=await rpc('eth_sendRawTransaction',[signed.rawTransaction]);
            if (lower(returned)!==lower(hash)) throw Error('Node returned an unexpected transaction hash.');
            for(let i=0;i<150;i++) {
                onProgress('Waiting for confirmation: '+hash);
                const receipt=await rpc('eth_getTransactionReceipt',[hash]);
                if(receipt) { if(BigInt(receipt.status)!==1n)throw Error('Mail transaction reverted: '+hash); return hash; }
                await new Promise(resolve=>setTimeout(resolve,2000));
            }
            throw Error('Confirmation timed out. Check the recorded transaction before retrying.');
        };
        const nonce=async()=>{
            const latest=await rpc('eth_getTransactionCount',[address,'latest']);
            if(BigInt(await rpc('eth_getTransactionCount',[address,'pending']))!==BigInt(latest))throw Error('The account has a pending transaction. Wait before retrying.');
            return latest;
        };
        const execute=async(plan,recipient,total,label)=>{
            if(!await api.shieldedV2Active())throw Error('Shield2 must be active.');
            const health=await prover.health();
            if(!health.ok || !health.hasProvingKeyV2 || !health.withdrawalBuildReady)throw Error('Local Mail proof builder is not ready.');
            const gasPrice=BigInt(await rpc('eth_gasPrice',[])), gasCost=GAS*gasPrice;
            await scanShieldedNotes(api,identity,state,(block,latest)=>onProgress(`Scanning notes: ${block} / ${latest}`));
            const budget=planMailFunding(state.notes,total,gasCost,BigInt(await rpc('eth_getBalance',[address,'latest'])),await api.shieldedGasSponsorActive());
            // Review the complete gas budget before any funding or withdrawal transaction.
            if(!onReview || !await onReview({label,recipient,amount:ethers.formatEther(total),gasPerTransaction:ethers.formatEther(gasCost),maxGas:ethers.formatEther(budget.maxGas),transactions:budget.transactions}))throw Error('Operation cancelled.');
            let remaining=total;
            while(remaining>0n) {
                await scanShieldedNotes(api,identity,state,(block,latest)=>onProgress(`Scanning notes: ${block} / ${latest}`));
                const balance=BigInt(await rpc('eth_getBalance',[address,'latest']));
                const sponsor=balance<gasCost ? gasCost-balance : 0n;
                if(sponsor && !await api.shieldedGasSponsorActive())throw Error('Public balance cannot cover network gas.');
                let note=state.notes.find(n=>n.status==='available' && Number(n.version)===2 && BigInt(n.assetId)===1n && BigInt(n.noteValueWei)>sponsor);
                if(!note) {
                    const value=remaining<MAX_NOTE ? remaining : MAX_NOTE;
                    if(balance<value+2n*gasCost)throw Error('Insufficient public balance for self-shielding and Mail gas.');
                    const n=await nonce();onProgress('Creating a private note for Mail…');
                    const response=await prover.buildDeposit({requestId:'mail-fund-'+ethers.hexlify(ethers.randomBytes(16)),from:address,to:address,amountWei:value.toString(),assetId:'1',ownerSecret:BigInt(address).toString(),recipientViewKey:ethers.hexlify(identity.viewPublicKey),nonce:n,gasPriceWei:gasPrice.toString()});
                    const tx=validateResponse(response,{nonce:n,gasPrice,part:{note:null,value,sponsor:0n},address,recipient:identity});
                    await broadcast(tx);continue;
                }
                const capacity=BigInt(note.noteValueWei)-sponsor, amount=remaining<capacity ? remaining : capacity;
                if((await api.privacyNullifierStatus(note.nullifier))?.spent) {note.status='spent';continue;}
                const path=await api.privacyCommitmentPath(note.commitment);
                if(!path.found)throw Error('Mail input note is no longer confirmed.');
                note={...note,merkleRoot:path.root,merklePath:path.merklePath.map(String),merklePathIndex:path.merklePathIndex.map(v=>BigInt(v).toString())};
                const n=await nonce();onProgress('Building '+label+' proof…');
                const response=await prover.buildWithdrawal({requestId:'mail-'+ethers.hexlify(ethers.randomBytes(16)),applicationData:plan.applicationData,from:address,to:recipient,amountWei:amount.toString(),changeViewKey:ethers.hexlify(identity.viewPublicKey),nonce:n,gasPriceWei:gasPrice.toString(),note});
                const tx=validateMailWithdrawal(response,{nonce:n,gasPrice,note,amount,sponsor,recipient,address,applicationData:plan.applicationData});
                await broadcast(tx);remaining-=amount;
                const spent=state.notes.find(x=>x.nullifier===note.nullifier);if(spent)spent.status='spent';
            }
        };
        const publish=async mailbox=>{
            const record=await ensureOwner(mailbox);
            if(lower(record.encryptionKey)===lower(ownKey))return;
            if(record.encryptionKey && record.encryptionKey!=='0x')throw Error('This mailbox uses a different mail key. Refusing to replace it and lose access to existing mail.');
            const plan=await rpc('emailvm_publishKey',[mailbox,ownKey]);assertMailPlan(plan,{kind:'key',mailbox,key:ownKey.slice(2)});
            await execute(plan,address,1n,'Publish encryption key for '+mailbox);
            const key=await rpc('emailvm_key',[mailbox]);if(lower(key.publicKey)!==lower(ownKey))throw Error('Encryption key is not yet indexed. Check publication before sending.');
        };
        if(operation==='buy') {
            const username=canonical(params.username),domain=canonical(params.domain).replace(/^@/,''),mailbox=username+'@'+domain;
            const record=await rpc('tkmdomain_domain',[domain]);
            const recipient=ethers.getAddress(record.payoutAddress && record.payoutAddress!==ethers.ZeroAddress ? record.payoutAddress : record.operator);
            const price=BigInt(await rpc('tkmdomain_subscriberUnitPrice',[]));
            const plan=await rpc('tkmdomain_buy',[username,domain]);
            const registryHash=emailRegistryHash('mailbox',mailbox);
            assertMailPlan(plan,{kind:'buy',username,domain,registryHash});
            if(lower(plan.registryHash)!==lower(registryHash)||lower(plan.withdrawalRecipient)!==lower(recipient)||BigInt(plan.totalWithdrawalAmountWei)!==price)throw Error('Mailbox payment differs from the domain price or payout address.');
            const pending=await rpc('tkmdomain_pending',[]);
            const prior=(pending||[]).find(p=>p.kind==='buy'&&p.domain===domain&&p.username===username&&lower(p.payer)===lower(address)&&lower(p.recipient)===lower(recipient)&&BigInt(p.required)===price);
            const remaining=price-BigInt(prior?.paid||0);
            if(remaining<=0n)throw Error('Mailbox payment is awaiting indexing. Check registration before retrying.');
            await execute(plan,recipient,remaining,'Register '+mailbox);
            await ensureOwner(mailbox); await publish(mailbox);
        } else if(operation==='publish') {
            await publish(canonical(params.mailbox));
        } else if(operation==='send') {
            const from=canonical(params.from),to=canonical(params.to);
            const body=String(params.subject||'').trim()+'\n\n'+String(params.body||'');
            if(!body.trim()||ethers.toUtf8Bytes(body).length>4000)throw Error('Write a message of at most 4,000 UTF-8 bytes including subject.');
            const own=await ensureOwner(from);
            if(lower(own.encryptionKey)!==lower(ownKey))throw Error('Publish this wallet’s Mail encryption key before sending.');
            const peer=await rpc('emailvm_key',[to]);
            const encrypted=encryptEmailMessage(email.privateKey,ethers.getBytes(peer.publicKey),from,to,body);
            const ciphertext=ethers.hexlify(encrypted.ciphertext),n=ethers.hexlify(encrypted.nonce);
            const plan=await rpc('emailvm_send',[from,to,ciphertext,n]);
            assertMailPlan(plan,{kind:'message',from,to,ciphertext:ciphertext.slice(2),nonce:n.slice(2)});
            await execute(plan,address,1n,'Send encrypted mail to '+to);
        } else if(operation==='decrypt') {
            const mailbox=canonical(params.mailbox),message=params.message;
            const own=await ensureOwner(mailbox);
            if(lower(own.encryptionKey)!==lower(ownKey))throw Error('This wallet does not hold the published Mail key.');
            const from=canonical(message.from),to=canonical(message.to);
            if(from!==mailbox&&to!==mailbox)throw Error('Message does not belong to this mailbox.');
            const peer=await rpc('emailvm_key',[from===mailbox?to:from]);
            return decryptEmailMessage(email.privateKey,ethers.getBytes(peer.publicKey),from,to,ethers.getBytes(message.ciphertext),ethers.getBytes(message.nonce));
        } else throw Error('Unsupported Mail operation.');
        return hashes;
    } catch(error) {
        if(hashes.length)error.message+=' Attempted transaction hashes: '+hashes.join(', ')+'. Check before retrying.';
        throw error;
    } finally {seed?.fill(0);clearShieldedIdentity(identity);clearEmailIdentity(email);release();}
}
