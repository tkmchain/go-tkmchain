import { ethers } from 'ethers';

const REGISTRY_PREFIX = 'TKM_EMAILVM_REGISTRY_V1';

export function emailRegistryHash(kind, canonicalName) {
    if (kind !== 'domain' && kind !== 'mailbox') throw new Error('invalid EmailVM registry kind');
    if (!canonicalName || canonicalName !== canonicalName.toLowerCase()) throw new Error('EmailVM registry names must be canonical lowercase values');
    return ethers.keccak256(ethers.toUtf8Bytes(`${REGISTRY_PREFIX}\0${kind}\0${canonicalName}`));
}
