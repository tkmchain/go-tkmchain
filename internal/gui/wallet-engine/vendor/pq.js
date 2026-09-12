import { ethers } from 'ethers';
import { ctr } from '@noble/ciphers/aes.js';
import { ml_dsa87 } from '@noble/post-quantum/ml-dsa.js';
import { getPQAddressFromPublicKey, PQ_ALGORITHM, PQ_PUBLIC_KEY_BYTES } from './keystore.js';

const PQ_TX_TYPE = 0x06;
const DEFAULT_SCRYPT_N = 1 << 17;
const SCRYPT_R = 8;
const SCRYPT_P = 1;
const DERIVED_KEY_BYTES = 32;
const PQ_SEED_BYTES = 32;

function bytes(value, name) {
    try {
        return ethers.getBytes(value);
    } catch {
        throw new Error(`${name} is not valid hexadecimal data`);
    }
}

function cryptoSection(keystore) {
    const section = keystore?.crypto || keystore?.Crypto;
    if (!section) throw new Error('PQ keyfile is missing its crypto section');
    return section;
}

function equalBytes(left, right) {
    if (left.length !== right.length) return false;
    let difference = 0;
    for (let i = 0; i < left.length; i++) difference |= left[i] ^ right[i];
    return difference === 0;
}

async function deriveKey(section, password) {
    const kdf = String(section.kdf || '').toLowerCase();
    const params = section.kdfparams || {};
    const salt = bytes('0x' + String(params.salt || '').replace(/^0x/, ''), 'KDF salt');
    const passwordBytes = ethers.toUtf8Bytes(password);
    const dkLen = Number(params.dklen);
    if (dkLen !== DERIVED_KEY_BYTES) throw new Error('PQ keyfile KDF must derive 32 bytes');

    if (kdf === 'scrypt') {
        const n = Number(params.n);
        const r = Number(params.r);
        const p = Number(params.p);
        if (!Number.isSafeInteger(n) || n <= 1 || (n & (n - 1)) !== 0 || r <= 0 || p <= 0) {
            throw new Error('PQ keyfile has invalid scrypt parameters');
        }
        return bytes(await ethers.scrypt(passwordBytes, salt, n, r, p, dkLen), 'derived key');
    }
    if (kdf === 'pbkdf2') {
        const count = Number(params.c);
        const algorithm = String(params.prf || '').toLowerCase().split('-').pop();
        if (!Number.isSafeInteger(count) || count <= 0 || (algorithm !== 'sha256' && algorithm !== 'sha512')) {
            throw new Error('PQ keyfile has invalid PBKDF2 parameters');
        }
        return bytes(ethers.pbkdf2(passwordBytes, salt, count, dkLen, algorithm), 'derived key');
    }
    throw new Error(`unsupported PQ keyfile KDF: ${kdf || 'missing'}`);
}

function validatePublicIdentity(keystore, seed) {
    const generated = ml_dsa87.keygen(seed);
    const declaredPublicKey = bytes('0x' + String(keystore.publicKey || '').replace(/^0x/, ''), 'PQ public key');
    try {
        if (declaredPublicKey.length !== PQ_PUBLIC_KEY_BYTES || !equalBytes(generated.publicKey, declaredPublicKey)) {
            throw new Error('PQ keyfile public key does not match its encrypted seed');
        }
        const derivedAddress = getPQAddressFromPublicKey(ethers.hexlify(generated.publicKey));
        const declaredAddress = ethers.getAddress('0x' + String(keystore.address || '').replace(/^0x/, ''));
        if (derivedAddress !== declaredAddress) {
            throw new Error('PQ keyfile address does not match its encrypted seed');
        }
        return generated;
    } catch (error) {
        generated.secretKey.fill(0);
        throw error;
    }
}

export async function decryptPQKeystore(keystore, password) {
    if (Number(keystore?.version) !== 4 || keystore?.algorithm !== PQ_ALGORITHM) {
        throw new Error('a version-4 ML-DSA-87 keyfile is required');
    }
    if (typeof password !== 'string' || password.length === 0) {
        throw new Error('PQ keyfile password is required');
    }
    const section = cryptoSection(keystore);
    if (String(section.cipher || '').toLowerCase() !== 'aes-128-ctr') {
        throw new Error('PQ keyfile cipher must be aes-128-ctr');
    }
    const derivedKey = await deriveKey(section, password);
    const ciphertext = bytes('0x' + String(section.ciphertext || '').replace(/^0x/, ''), 'ciphertext');
    const expectedMac = String(section.mac || '').replace(/^0x/, '').toLowerCase();
    const actualMac = ethers.keccak256(ethers.concat([derivedKey.slice(16, 32), ciphertext])).slice(2);
    if (actualMac !== expectedMac) {
        derivedKey.fill(0);
        throw new Error('incorrect PQ keyfile password');
    }
    const iv = bytes('0x' + String(section.cipherparams?.iv || '').replace(/^0x/, ''), 'cipher IV');
    const seed = ctr(derivedKey.slice(0, 16), iv).decrypt(ciphertext);
    derivedKey.fill(0);
    if (seed.length !== PQ_SEED_BYTES) {
        seed.fill(0);
        throw new Error(`PQ keyfile seed has ${seed.length} bytes, want ${PQ_SEED_BYTES}`);
    }
    try {
        const generated = validatePublicIdentity(keystore, seed);
        generated.secretKey.fill(0);
        return seed;
    } catch (error) {
        seed.fill(0);
        throw error;
    }
}

export async function createPQKeystore(password, options = {}) {
    if (typeof password !== 'string' || password.length < 6) {
        throw new Error('password must be at least 6 characters');
    }
    const seed = options.seed ? new Uint8Array(options.seed) : ethers.randomBytes(PQ_SEED_BYTES);
    if (seed.length !== PQ_SEED_BYTES) throw new Error('ML-DSA-87 seed must be 32 bytes');
    const generated = ml_dsa87.keygen(seed);
    let derivedKey;
    try {
        const publicKey = ethers.hexlify(generated.publicKey);
        const address = getPQAddressFromPublicKey(publicKey);
        const salt = options.salt ? new Uint8Array(options.salt) : ethers.randomBytes(32);
        const iv = options.iv ? new Uint8Array(options.iv) : ethers.randomBytes(16);
        const n = Number(options.scryptN || DEFAULT_SCRYPT_N);
        derivedKey = bytes(await ethers.scrypt(ethers.toUtf8Bytes(password), salt, n, SCRYPT_R, SCRYPT_P, DERIVED_KEY_BYTES), 'derived key');
        const ciphertext = ctr(derivedKey.slice(0, 16), iv).encrypt(seed);
        const mac = ethers.keccak256(ethers.concat([derivedKey.slice(16, 32), ciphertext])).slice(2);
        const idBytes = options.idBytes ? new Uint8Array(options.idBytes) : ethers.randomBytes(16);
        return {
            address: address.slice(2).toLowerCase(),
            algorithm: PQ_ALGORITHM,
            publicKey: publicKey.slice(2),
            crypto: {
                cipher: 'aes-128-ctr',
                ciphertext: ethers.hexlify(ciphertext).slice(2),
                cipherparams: { iv: ethers.hexlify(iv).slice(2) },
                kdf: 'scrypt',
                kdfparams: {
                    n,
                    r: SCRYPT_R,
                    p: SCRYPT_P,
                    dklen: DERIVED_KEY_BYTES,
                    salt: ethers.hexlify(salt).slice(2)
                },
                mac
            },
            id: ethers.uuidV4(idBytes),
            version: 4
        };
    } finally {
        seed.fill(0);
        generated.secretKey.fill(0);
        if (derivedKey) derivedKey.fill(0);
    }
}

function rlpUint(value, name) {
    const integer = ethers.getUint(value, name);
    return integer === 0n ? '0x' : ethers.toBeHex(integer);
}

function encodeAccessList(accessList = []) {
    return accessList.map((entry) => {
        const address = Array.isArray(entry) ? entry[0] : entry.address;
        const storageKeys = Array.isArray(entry) ? entry[1] : entry.storageKeys;
        return [ethers.getAddress(address), (storageKeys || []).map((key) => ethers.hexlify(key))];
    });
}

export async function signPQTkmTransaction(tx, keystore, password) {
    const seed = await decryptPQKeystore(keystore, password);
    try {
        return signPQTkmTransactionWithSeed(tx, seed);
    } finally {
        seed.fill(0);
    }
}

export function signPQMessageWithSeed(message, seed) {
    const generated = ml_dsa87.keygen(seed);
    try {
    const msg = message instanceof Uint8Array ? message : ethers.getBytes(message);
    const signature = ml_dsa87.sign(msg, generated.secretKey, { extraEntropy: false });
    if (!ml_dsa87.verify(signature, msg, generated.publicKey)) {
        throw new Error('local ML-DSA-87 message signature verification failed');
    }
    return { signature: ethers.hexlify(signature), publicKey: ethers.hexlify(generated.publicKey) };
    } finally { generated.secretKey.fill(0); }
}

export function signPQTkmTransactionWithSeed(tx, seed) {
    const generated = ml_dsa87.keygen(seed);
    const publicKey = ethers.hexlify(generated.publicKey);
    const fields = [
        rlpUint(tx.chainId, 'chainId'),
        rlpUint(tx.nonce, 'nonce'),
        rlpUint(tx.gasTipCap ?? tx.gasPrice, 'gasTipCap'),
        rlpUint(tx.gasFeeCap ?? tx.gasPrice, 'gasFeeCap'),
        rlpUint(tx.gas, 'gas'),
        tx.to == null ? '0x' : ethers.getAddress(tx.to),
        rlpUint(tx.value ?? 0, 'value'),
        ethers.hexlify(tx.data ?? tx.input ?? '0x'),
        encodeAccessList(tx.accessList),
        ethers.hexlify(ethers.toUtf8Bytes(PQ_ALGORITHM)),
        publicKey
    ];
    const signingHash = ethers.keccak256(ethers.concat([
        ethers.toBeHex(PQ_TX_TYPE, 1),
        ethers.encodeRlp(fields)
    ]));
    try {
        const signature = ml_dsa87.sign(ethers.getBytes(signingHash), generated.secretKey, { extraEntropy: false });
        if (!ml_dsa87.verify(signature, ethers.getBytes(signingHash), generated.publicKey)) {
            throw new Error('local ML-DSA-87 signature verification failed');
        }
        const rawTransaction = ethers.concat([
            ethers.toBeHex(PQ_TX_TYPE, 1),
            ethers.encodeRlp([...fields, ethers.hexlify(signature)])
        ]);
        return {
            rawTransaction,
            transactionHash: ethers.keccak256(rawTransaction),
            signingHash,
            publicKey
        };
    } finally {
        generated.secretKey.fill(0);
    }
}
