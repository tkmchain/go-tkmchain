//! Bounded embedded interface. It accepts the same canonical protocol as CLI.
use super::*;

fn words(input: &[u8]) -> Result<Vec<u64>, String> {
    if !input.len().is_multiple_of(8) {
        return Err("invalid word length".into());
    }
    let words = input
        .chunks_exact(8)
        .map(|v| u64::from_le_bytes(v.try_into().unwrap()))
        .collect::<Vec<_>>();
    canonical_words(&words, words.len())?;
    Ok(words)
}
fn bytes(words: &[u64]) -> Vec<u8> {
    words.iter().flat_map(|v| v.to_le_bytes()).collect()
}
fn process(operation: u32, data: &[u8]) -> Result<Vec<u8>, String> {
    if operation == 11 || operation == 12 {
        const MAGIC: &[u8] = b"TKMPTVM1";
        let public_count = PRIVATE_TVM_PUBLIC_WORDS;
        let prefix = MAGIC.len() + public_count * 8;
        if data.len() < prefix || &data[..MAGIC.len()] != MAGIC {
            return Err("invalid private TVM request".into());
        }
        let public = words(&data[MAGIC.len()..prefix])?;
        let input = &data[prefix..];
        if operation == 12 {
            if input.len() < 4 {
                return Err("truncated private TVM proof".into());
            }
            let count = u32::from_le_bytes(input[..4].try_into().unwrap()) as usize;
            if count == 0 || count > MAX_PROOF_WORDS || input.len() != 4 + count * 8 {
                return Err("invalid private TVM proof length".into());
            }
            verify_private_tvm(&public, &words(&input[4..])?)?;
            return Ok(b"OK\n".to_vec());
        }
        if input.len() != (PRIVATE_TVM_SECRET_WORDS + PRIVATE_TVM_PATH_DIGESTS * 5) * 8 {
            return Err("invalid private TVM witness length".into());
        }
        let mut secret = words(&input[..PRIVATE_TVM_SECRET_WORDS * 8])?;
        let path = words(&input[PRIVATE_TVM_SECRET_WORDS * 8..])?
            .chunks_exact(5)
            .map(|d| d.try_into().unwrap())
            .collect::<Vec<_>>();
        let proof = prove_private_tvm(&public, &secret, &path)?;
        secret.fill(0);
        let mut out = (proof.len() as u32).to_le_bytes().to_vec();
        out.extend(bytes(&proof));
        return Ok(out);
    }
    if operation == 4 {
        let w = words(data)?;
        if w.len() != 10 {
            return Err("invalid pair".into());
        }
        let left = Digest::new(
            w[..5]
                .iter()
                .copied()
                .map(BFieldElement::new)
                .collect::<Vec<_>>()
                .try_into()
                .unwrap(),
        );
        let right = Digest::new(
            w[5..]
                .iter()
                .copied()
                .map(BFieldElement::new)
                .collect::<Vec<_>>()
                .try_into()
                .unwrap(),
        );
        return Ok(bytes(
            &Tip5::hash_pair(left, right).values().map(|v| v.value()),
        ));
    }
    if operation == 5 {
        let w = words(data)?;
        if w.is_empty() || w.len() > 40 {
            return Err("invalid hash input".into());
        }
        return Ok(bytes(
            &Tip5::hash_varlen(&w.into_iter().map(BFieldElement::new).collect::<Vec<_>>())
                .values()
                .map(|v| v.value()),
        ));
    }
    let v4 = (8..=10).contains(&operation);
    let ownership = operation == 6 || operation == 7;
    let public_count = if ownership {
        OWNER_PUBLIC_WORDS
    } else if v4 {
        PUBLIC_WORDS_V4
    } else {
        PUBLIC_WORDS
    };
    let prefix = 8 + public_count * 8;
    let expected_magic = if ownership {
        b"TKMS3OWN" as &[u8]
    } else if v4 {
        b"TKMS4STK" as &[u8]
    } else {
        b"TKMS3STK" as &[u8]
    };
    if data.len() < prefix || &data[..8] != expected_magic {
        return Err("invalid request".into());
    }
    let public = words(&data[8..prefix])?;
    let input = &data[prefix..];
    if operation == 1 || operation == 7 || operation == 8 {
        if input.len() < 4 {
            return Err("truncated proof".into());
        }
        let count = u32::from_le_bytes(input[..4].try_into().unwrap()) as usize;
        if count == 0 || count > MAX_PROOF_WORDS || input.len() != 4 + count * 8 {
            return Err("invalid proof length".into());
        }
        if ownership {
            verify_owner(&public, &words(&input[4..])?)?;
        } else if operation == 8 {
            verify_spend_v4(&public, &words(&input[4..])?)?;
        } else {
            verify_spend(&public, &words(&input[4..])?)?;
        }
        return Ok(b"OK\n".to_vec());
    }
    if operation == 6 {
        let mut secret = words(input)?;
        let result = prove_owner(&public, &secret).map(|p| {
            let mut out = (p.len() as u32).to_le_bytes().to_vec();
            out.extend(bytes(&p));
            out
        });
        secret.fill(0);
        return result;
    }
    if input.len() != (SECRET_WORDS + PATH_DIGESTS * 5) * 8 {
        return Err("invalid witness length".into());
    }
    let mut secret = words(&input[..SECRET_WORDS * 8])?;
    let path = words(&input[SECRET_WORDS * 8..])?
        .chunks_exact(5)
        .map(|d| d.try_into().unwrap())
        .collect::<Vec<_>>();
    let result = match operation {
        2 => prove_spend(&public, &secret, &path).map(|p| {
            let mut out = (p.len() as u32).to_le_bytes().to_vec();
            out.extend(bytes(&p));
            out
        }),
        9 => prove_spend_v4(&public, &secret, &path).map(|p| {
            let mut out = (p.len() as u32).to_le_bytes().to_vec();
            out.extend(bytes(&p));
            out
        }),
        3 => describe_spend(&public, &secret, &path).map(|d| bytes(&d)),
        10 => describe_spend_v4(&public, &secret, &path).map(|d| bytes(&d)),
        _ => Err("invalid operation".into()),
    };
    secret.fill(0);
    result
}

/// # Safety
/// Input/output pointers must refer to non-overlapping live buffers of the
/// supplied lengths. written must point to a writable usize. No pointer is kept.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn tkm_shield3_call(
    operation: u32,
    input: *const u8,
    length: usize,
    output: *mut u8,
    capacity: usize,
    written: *mut usize,
) -> i32 {
    if input.is_null()
        || output.is_null()
        || written.is_null()
        // Shield4 adds five public field words for its linkability tag. Keep
        // the native ABI bound large enough for that envelope while retaining
        // the same fixed proof-size cap.
        || length > 8 + PUBLIC_WORDS_V4 * 8 + 4 + MAX_PROOF_WORDS * 8
        || capacity > 4 + MAX_PROOF_WORDS * 8
    {
        return 1;
    }
    unsafe {
        *written = 0;
    }
    let result = std::panic::catch_unwind(|| {
        process(operation, unsafe {
            std::slice::from_raw_parts(input, length)
        })
    });
    match result {
        Ok(Ok(mut data)) if data.len() <= capacity => {
            unsafe {
                std::ptr::copy_nonoverlapping(data.as_ptr(), output, data.len());
                *written = data.len();
            }
            data.fill(0);
            0
        }
        _ => 1,
    }
}
