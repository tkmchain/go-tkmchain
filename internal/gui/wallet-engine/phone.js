import {ethers} from 'ethers';
import {ml_dsa87} from '@noble/post-quantum/ml-dsa.js';
import {decryptPQKeystore} from './vendor/pq.js';
import {normalizeKeystore} from './vendor/keystore.js';
import {derivePhoneEncryptionSeed, encryptPhoneV2, phoneEncryptionPublicKey as phoneEncryptionPublicKeyFromSeed, decryptPhoneV2} from './phone-crypto.js';
const prefix = ethers.toUtf8Bytes('TKMPHONE_PQ_V1');
export function signPhoneDigestWithSeed(seed, digest) {
    const message = ethers.getBytes(digest);
    if (message.length !== 32) throw Error('Phone signing hash must be 32 bytes.');
    const keys = ml_dsa87.keygen(seed);
    try {
        const signature = ml_dsa87.sign(ethers.getBytes(ethers.concat([prefix, message])), keys.secretKey);
        return ethers.hexlify(ethers.concat([prefix, keys.publicKey, signature]));
    } finally { keys.secretKey.fill(0); }
}
export async function signPhoneDigest(keystore, password, digest) {
    const seed = await decryptPQKeystore(normalizeKeystore(keystore), password);
    try { return signPhoneDigestWithSeed(seed, digest); } finally { seed.fill(0); }
}
export async function phonePublicKey(keystore, password) {
    const normalized = normalizeKeystore(keystore);
    const seed = await decryptPQKeystore(normalized, password);
    try { return '0x' + normalized.publicKey.replace(/^0x/, ''); } finally { seed.fill(0); }
}


export async function phoneEncryptionPublicKey(keystore, password) {
    const seed = await decryptPQKeystore(normalizeKeystore(keystore), password);
    try { return ethers.hexlify(phoneEncryptionPublicKeyFromSeed(derivePhoneEncryptionSeed(seed))); }
    finally { seed.fill(0); }
}

export async function phoneEncryptV2(publicKey, chainId, from, to, nonce, plaintext) {
    return encryptPhoneV2(publicKey, chainId, from, to, nonce, plaintext);
}

export async function phoneDecryptV2(keystore, password, chainId, from, to, nonce, ciphertext) {
    const seed = await decryptPQKeystore(normalizeKeystore(keystore), password);
    try { return ethers.hexlify(decryptPhoneV2(derivePhoneEncryptionSeed(seed), ciphertext, chainId, from, to, nonce)); }
    finally { seed.fill(0); }
}
