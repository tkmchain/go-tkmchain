import {ethers} from 'ethers';
import {decryptPQKeystore} from './vendor/pq.js';
import {normalizeKeystore} from './vendor/keystore.js';
import {decodeShieldedPaymentCode, deriveShieldedIdentity, clearShieldedIdentity, scanShieldedNotes, shieldedBalance} from './vendor/shielded.js';
import {TkmChainAPI} from './vendor/api.js';
import {sendTKM as sendV2} from './shield2-send.js';
import {shield3Status,sendShield3,shield3Scan} from './shield3.js';
export {shield3Status,shield3Identity,shield3ViewKeys,shield3ViewScan,shield3ViewStamp,shield3FetchRelayOffer,shield3Scan,shield3Funds, shield3RegisterStamp,shield3StampSponsorship,shield3Relay,shield3ReviewRelayOffer,shield3Disclosure,validateShield3Amount,validateShield3Payments,validateShield3Recipient} from './shield3.js';
export async function sendTKM(options){const status=await shield3Status(options);return status.active?sendShield3(options):sendV2(options)}
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
 if((await shield3Status({rpcURL,rpcToken})).active){const result=await shield3Scan({keystore,password,rpcToken});return ethers.formatEther(result.balanceWei)}
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
export {signPhoneDigest, phonePublicKey, phoneEncryptionPublicKey, phoneEncryptV2, phoneDecryptV2} from './phone.js';
export {runMailOperation} from './mail.js';

export {migrateShield2Note} from './shield3-migrate.js';
