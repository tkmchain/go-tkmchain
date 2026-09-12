import {ethers} from 'ethers';
import {decryptPQKeystore} from './vendor/pq.js';
import {normalizeKeystore} from './vendor/keystore.js';
import {decodeShieldedPaymentCode, deriveShieldedIdentity, clearShieldedIdentity, scanShieldedNotes, shieldedBalance} from './vendor/shielded.js';
import {TkmChainAPI} from './vendor/api.js';
export {sendTKM} from './shield2-send.js';
export {decodeShieldedPaymentCode};

// TKM PQ recovery v1 encodes the raw 32-byte ML-DSA seed as 24 BIP39 words.
// It is not Ethereum HD derivation and does not use a BIP39 passphrase.
export function newRecoveryPhrase() { return ethers.Mnemonic.fromEntropy(ethers.randomBytes(32)).phrase; }
export function seedFromPhrase(phrase) {
    const normalized = String(phrase).normalize('NFKD').trim().toLowerCase().split(/\s+/).join(' ');
    if (normalized.split(' ').length !== 24) throw new Error('Enter all 24 recovery words.');
    const mnemonic = ethers.Mnemonic.fromPhrase(normalized);
    if (ethers.getBytes(mnemonic.entropy).length !== 32) throw new Error('Expected a 24-word TKM PQ recovery phrase.');
    return mnemonic.entropy;
}
export async function recoveryPhraseForKeyfile(keystore, password) {
    const seed = await decryptPQKeystore(normalizeKeystore(keystore), password);
    try { return ethers.Mnemonic.fromEntropy(seed).phrase; } finally { seed.fill(0); }
}
export function keyfileFromHex(hex) { return JSON.parse(ethers.toUtf8String(hex)); }
export function validateRecipient(code) { return decodeShieldedPaymentCode(String(code).trim(),8979); }
export async function scanWallet({keystore,password,rpcURL,rpcToken,onProgress}) {
    const normalized=normalizeKeystore(keystore), seed=await decryptPQKeystore(normalized,password);
    let identity,provider;
    try {
        identity=deriveShieldedIdentity(seed,ethers.getAddress('0x'+normalized.address.replace(/^0x/,'')),8979);
        const connection=new ethers.FetchRequest(rpcURL);connection.setHeader('X-GUI-Token',rpcToken);
        provider=new ethers.JsonRpcProvider(connection);
        if (BigInt(await provider.send('eth_chainId',[]))!==8979n) throw new Error('This wallet requires TKM mainnet.');
        const state={lastScannedBlock:-1,notes:[]};
        await scanShieldedNotes(new TkmChainAPI(provider),identity,state,onProgress);
        return ethers.formatEther(shieldedBalance(state.notes));
    } finally {seed.fill(0);clearShieldedIdentity(identity);provider?.destroy();}
}
export {signPhoneDigest, phonePublicKey} from './phone.js';
export {runMailOperation} from './mail.js';
