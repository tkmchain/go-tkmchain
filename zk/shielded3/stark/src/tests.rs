use super::*;

fn hash(words: &[u64]) -> [u64; 5] {
    Tip5::hash_varlen(
        &words
            .iter()
            .copied()
            .map(BFieldElement::new)
            .collect::<Vec<_>>(),
    )
    .values()
    .map(|v| v.value())
}
fn note(chain_asset: &[u64], owner: &[u64], random: &[u64], value: &[u64]) -> [u64; 5] {
    let mut words = vec![DOMAIN_NOTE];
    words.extend(chain_asset);
    words.extend(owner);
    words.extend(value);
    words.extend(random);
    hash(&words)
}
fn one_time(owner: &[u64], random: &[u64], commitment: &[u64]) -> [u64; 5] {
    let mut words = vec![DOMAIN_ONETIME_KEY];
    words.extend(owner);
    words.extend(random);
    words.extend(commitment);
    hash(&words)
}
fn pair(left: [u64; 5], right: [u64; 5]) -> [u64; 5] {
    Tip5::hash_pair(
        Digest::new(left.map(BFieldElement::new)),
        Digest::new(right.map(BFieldElement::new)),
    )
    .values()
    .map(|v| v.value())
}
fn fixture() -> (Vec<u64>, Vec<u64>, Vec<[u64; 5]>) {
    let mut public = vec![0; PUBLIC_WORDS];
    public[0] = 8979;
    public[2] = 1;
    public[87] = 1;
    public[4] = 9;
    for (i, word) in public.iter_mut().enumerate().take(28).skip(12) {
        *word = (i * 101) as u64;
    }
    let mut secret = vec![0; SECRET_WORDS];
    secret[..10].copy_from_slice(&[11, 22, 33, 44, 55, 66, 77, 88, 99, 111]);
    secret[10] = u32::MAX as u64 - 5;
    secret[11] = 3;
    secret[18] = 0x87654321;
    let owner = hash(&[DOMAIN_OWNER, 11, 22, 33, 44, 55]);
    let input = note(&public[..4], &owner, &secret[5..10], &secret[10..18]);
    let mut nullifier = vec![DOMAIN_NULLIFIER];
    nullifier.extend(&public[..4]);
    nullifier.extend(hash(&[DOMAIN_NULLIFIER_KEY, 11, 22, 33, 44, 55]));
    nullifier.extend(&secret[5..18]);
    public[33..38].copy_from_slice(&hash(&nullifier));
    let mut path = (0..MERKLE_DEPTH)
        .map(|i| hash(&[9999, i as u64]))
        .collect::<Vec<_>>();
    let mut root = input;
    let mut index = secret[18];
    for sibling in &path {
        root = if index & 1 == 0 {
            pair(root, *sibling)
        } else {
            pair(*sibling, root)
        };
        index >>= 1;
    }
    public[28..33].copy_from_slice(&root);
    // sum outputs + public value = (3<<32) + (u32::MAX-5), exercising carry.
    let values = [
        [u32::MAX as u64, 1],
        [u32::MAX as u64 - 13, 1],
        [0, 0],
        [0, 0],
    ];
    for i in 0..4 {
        let base = 19 + 18 * i;
        for j in 0..10 {
            secret[base + j] = (200 + i * 20 + j) as u64;
        }
        if i == 3 {
            secret[base..base + 5].copy_from_slice(&owner);
        }
        secret[base + 10..base + 12].copy_from_slice(&values[i]);
        let commitment = note(
            &public[..4],
            &secret[base..base + 5],
            &secret[base + 5..base + 10],
            &secret[base + 10..base + 18],
        );
        public[38 + 5 * i..43 + 5 * i].copy_from_slice(&commitment);
        public[88 + 5 * i..93 + 5 * i].copy_from_slice(&one_time(
            &secret[base..base + 5],
            &secret[base + 5..base + 10],
            &commitment,
        ));
    }
    path.extend(vec![[0; 5]; MERKLE_DEPTH * 3]);
    stamp_fixture(&mut public, &mut secret, &mut path);
    (public, secret, path)
}
fn run(public: &[u64], secret: &[u64], path: &[[u64; 5]]) -> bool {
    let tokens = canonical_words(secret, SECRET_WORDS).unwrap();
    let Ok(active) = spending_paths(public, path) else {
        return false;
    };
    let digests: Vec<_> = active
        .iter()
        .map(|p| Digest::new(p.map(BFieldElement::new)))
        .collect::<Vec<_>>();
    VM::run(
        spend_program(),
        PublicInput::new(canonical_words(public, PUBLIC_WORDS).unwrap()),
        NonDeterminism::new(tokens).with_digests(digests),
    )
    .is_ok()
}

fn run_v4(public: &[u64], secret: &[u64], path: &[[u64; 5]]) -> bool {
    let public = v4_public(public, secret);
    let tokens = canonical_words(secret, SECRET_WORDS).unwrap();
    let Ok(active) = spending_paths_v4(&public, path) else {
        return false;
    };
    let digests: Vec<_> = active
        .iter()
        .map(|p| Digest::new(p.map(BFieldElement::new)))
        .collect::<Vec<_>>();
    VM::run(
        spend_program_v4(),
        PublicInput::new(canonical_words(&public, PUBLIC_WORDS_V4).unwrap()),
        NonDeterminism::new(tokens).with_digests(digests),
    )
    .is_ok()
}

fn v4_public(public: &[u64], secret: &[u64]) -> Vec<u64> {
    let mut public = public.to_vec();
    let owner = hash(&[
        DOMAIN_OWNER,
        secret[0],
        secret[1],
        secret[2],
        secret[3],
        secret[4],
    ]);
    let mut tag = vec![DOMAIN_SHIELD4_TAG];
    tag.extend(owner);
    tag.extend(&secret[5..10]);
    tag.push(secret[18]);
    tag.extend(&public[28..33]);
    public.extend(hash(&tag));
    public
}

#[test]
fn shield4_tag_and_program_domain_are_bound() {
    let (public, secret, path) = fixture();
    assert!(
        run_v4(&public, &secret, &path),
        "valid Shield4 witness rejected"
    );
    let public_v4 = v4_public(&public, &secret);
    let proof = prove_spend_v4(&public_v4, &secret, &path).expect("real Shield4 STARK proving");
    verify_spend_v4(&public_v4, &proof).expect("real Shield4 STARK verification");
    if let Ok(directory) = std::env::var("TKM_SHIELD4_TESTDATA") {
        use std::io::Write;
        std::fs::create_dir_all(&directory).unwrap();
        for (name, data) in [
            ("public", public_v4),
            ("secret", secret.clone()),
            ("path", path.iter().flatten().copied().collect()),
            ("proof", proof),
        ] {
            let mut file = std::fs::File::create(format!("{directory}/{name}.bin")).unwrap();
            for value in data {
                file.write_all(&value.to_le_bytes()).unwrap();
            }
        }
    }

    let mut changed_secret = secret.clone();
    changed_secret[5] ^= 1;
    assert!(
        !run_v4(&public, &changed_secret, &path),
        "changed randomness accepted"
    );

    let mut changed_tag = public.clone();
    changed_tag[0] ^= 1;
    assert!(
        !run_v4(&changed_tag, &secret, &path),
        "changed anchor context accepted"
    );

    assert_ne!(
        spend_program().hash(),
        spend_program_v4().hash(),
        "Shield3 and Shield4 claims must use different frozen programs"
    );
}

#[test]
fn valid_spend_and_constraint_rejections() {
    let (public, secret, path) = fixture();
    assert!(
        run(&public, &secret, &path),
        "independently hashed valid witness rejected"
    );
    // All secret words have an algebraic role: ownership, commitment opening,
    // balance or index. Changing any one must invalidate the witness.
    for i in 0..SECRET_WORDS {
        let mut changed = secret.clone();
        changed[i] ^= 1;
        assert!(
            !run(&public, &changed, &path),
            "unconstrained secret word {i}"
        );
    }
    for i in 0..PATH_DIGESTS {
        for j in 0..5 {
            let mut changed = path.clone();
            changed[i][j] ^= 1;
            assert!(
                !run(&public, &secret, &changed),
                "unconstrained path {i}/{j}"
            );
        }
    }
    for i in (0..12).chain(28..59) {
        let mut changed = public.clone();
        changed[i] ^= 1;
        assert!(
            !run(&changed, &secret, &path),
            "unconstrained public word {i}"
        );
    }
    let mut oversized = secret.clone();
    oversized[10] = 1 << 32;
    assert!(
        !run(&public, &oversized, &path),
        "out of range input limb accepted"
    );
}

fn refresh_commitments(public: &mut [u64], secret: &[u64], path: &[[u64; 5]]) {
    let mut owner = vec![DOMAIN_OWNER];
    owner.extend(&secret[..5]);
    let input = note(&public[..4], &hash(&owner), &secret[5..10], &secret[10..18]);
    let mut root = input;
    let mut index = secret[18];
    for sibling in path.iter().take(MERKLE_DEPTH) {
        root = if index & 1 == 0 {
            pair(root, *sibling)
        } else {
            pair(*sibling, root)
        };
        index >>= 1;
    }
    public[28..33].copy_from_slice(&root);
    let mut nullifier = vec![DOMAIN_NULLIFIER];
    nullifier.extend(&public[..4]);
    let mut key = vec![DOMAIN_NULLIFIER_KEY];
    key.extend(&secret[..5]);
    nullifier.extend(hash(&key));
    nullifier.extend(&secret[5..18]);
    public[33..38].copy_from_slice(&hash(&nullifier));
    for i in 0..4 {
        let base = 19 + 18 * i;
        let out = note(
            &public[..4],
            &secret[base..base + 5],
            &secret[base + 5..base + 10],
            &secret[base + 10..base + 18],
        );
        public[38 + 5 * i..43 + 5 * i].copy_from_slice(&out);
        public[88 + 5 * i..93 + 5 * i].copy_from_slice(&one_time(
            &secret[base..base + 5],
            &secret[base + 5..base + 10],
            &out,
        ));
    }
}

#[test]
fn full_width_amounts_and_overflow_are_constrained() {
    let (mut public, mut secret, path) = fixture();
    public[4..12].fill(0);
    for i in 0..4 {
        secret[29 + 18 * i..37 + 18 * i].fill(0);
    }
    secret[10..18].fill(u32::MAX as u64);
    secret[83..91].fill(u32::MAX as u64);
    refresh_commitments(&mut public, &secret, &path);
    assert!(
        run(&public, &secret, &path),
        "full 256-bit exact amount rejected"
    );
    // All commitments and the root are valid for these openings. Only the
    // conservation check can reject 2^256+1 being wrapped to an input of 1.
    secret[10..18].fill(0);
    secret[10] = 1;
    secret[47] = 2;
    refresh_commitments(&mut public, &secret, &path);
    assert!(
        !run(&public, &secret, &path),
        "256-bit overflow minted value"
    );
    secret[29] = 1 << 32;
    refresh_commitments(&mut public, &secret, &path);
    assert!(
        !run(&public, &secret, &path),
        "out-of-range output limb accepted"
    );
}

#[test]
fn canonical_encoding_and_limits() {
    assert!(canonical_words(&[FIELD_MODULUS], 1).is_err());
    assert!(canonical_words(&[0], 2).is_err());
    assert!(verify_spend(&[0; PUBLIC_WORDS], &[0]).is_err());
    let (public, _, _) = fixture();
    for exponent in [16, 31, 32, 63, 64, u32::MAX] {
        let mut stream = triton_vm::proof_stream::ProofStream::new();
        stream.enqueue(triton_vm::proof_item::ProofItem::Log2PaddedHeight(exponent));
        let proof = Proof::from(stream);
        assert!(
            verify_spend(
                &public,
                &proof.0.iter().map(|v| v.value()).collect::<Vec<_>>()
            )
            .is_err()
        );
    }
    for bad in [vec![], vec![0], vec![FIELD_MODULUS], vec![1; 128]] {
        assert!(verify_spend(&public, &bad).is_err());
    }
    let mut oversized = public.clone();
    oversized[12] = 1 << 32;
    assert!(verify_spend(&oversized, &[1]).is_err());
}

#[test]
fn real_stark_roundtrip_tampering_and_replay() {
    let (public, secret, path) = fixture();
    let proof = prove_spend(&public, &secret, &path).expect("real STARK proving");
    verify_spend(&public, &proof).expect("real STARK verification");
    eprintln!(
        "real proof: {} bytes, {} words",
        proof.len() * 8 + 4,
        proof.len()
    );
    // Transaction intent is a public claim rather than private witness data.
    // Reusing the proof for any different transaction must fail.
    for i in 0..PUBLIC_WORDS {
        let mut changed = public.clone();
        changed[i] ^= 1;
        assert!(
            verify_spend(&changed, &proof).is_err(),
            "proof replay word {i}"
        );
    }
    for i in [0, 1, proof.len() / 4, proof.len() / 2, proof.len() - 1] {
        let mut changed = proof.clone();
        changed[i] ^= 1;
        assert!(
            verify_spend(&public, &changed).is_err(),
            "tampered proof word {i}"
        );
    }
    if let Ok(directory) = std::env::var("TKM_SHIELD3_TESTDATA") {
        use std::io::Write;
        std::fs::create_dir_all(&directory).unwrap();
        for (name, data) in [
            ("public", public),
            ("secret", secret),
            ("path", path.into_iter().flatten().collect()),
            ("proof", proof),
        ] {
            let mut file = std::fs::File::create(format!("{directory}/{name}.bin")).unwrap();
            for v in data {
                file.write_all(&v.to_le_bytes()).unwrap();
            }
        }
    }
}

#[test]
fn private_send_cap_and_change_owner() {
    let (mut public, mut secret, path) = fixture();
    public[4..12].fill(0);
    for i in 0..4 {
        secret[29 + 18 * i..37 + 18 * i].fill(0);
    }
    let cap = 5_000_000u128 * 1_000_000_000_000_000_000u128;
    for (value, expected) in [(cap - 1, true), (cap, true), (cap + 1, false)] {
        secret[10..18].fill(0);
        secret[29..37].fill(0);
        for limb in 0..4 {
            let v = ((value >> (32 * limb)) & 0xffffffff) as u64;
            secret[10 + limb] = v;
            secret[29 + limb] = v;
        }
        refresh_commitments(&mut public, &secret, &path);
        assert_eq!(
            run(&public, &secret, &path),
            expected,
            "cap boundary {value}"
        );
    }
    // Large self-owned change remains valid, but changing its owner does not.
    secret[29..37].fill(0);
    secret[10..18].fill(u32::MAX as u64);
    secret[83..91].fill(u32::MAX as u64);
    refresh_commitments(&mut public, &secret, &path);
    assert!(run(&public, &secret, &path));
    secret[73] ^= 1;
    refresh_commitments(&mut public, &secret, &path);
    assert!(!run(&public, &secret, &path));
}

#[test]
fn deposits_require_exact_public_funding() {
    let (mut public, mut secret, mut path) = fixture();
    path[..4 * MERKLE_DEPTH].fill([0; 5]);
    public[58] = 1;
    public[87] = 0;
    public[28..38].fill(0);
    public[4..12].copy_from_slice(&secret[10..18]);
    // fixture's output total is input minus the old nine-unit public release.
    secret[29] += 9;
    // propagate the resulting u32 carry.
    secret[30] += secret[29] >> 32;
    secret[29] &= 0xffffffff;
    refresh_commitments(&mut public, &secret, &path);
    public[28..38].fill(0);
    assert!(run(&public, &secret, &path));
    let proof = prove_spend(&public, &secret, &path).expect("real deposit STARK");
    verify_spend(&public, &proof).unwrap();
    public[4] += 1;
    assert!(!run(&public, &secret, &path));
    assert!(verify_spend(&public, &proof).is_err());
}

#[test]
fn fee_sponsorship_is_checked_and_excluded_from_send_cap() {
    let (mut public, secret, path) = fixture();
    public[59] = 9;
    assert!(run(&public, &secret, &path));
    public[59] = 10;
    assert!(!run(&public, &secret, &path));
    public[59] = 1 << 32;
    assert!(!run(&public, &secret, &path));
}

fn stamp_fixture(public: &mut [u64], secret: &mut [u64], path: &mut Vec<[u64; 5]>) {
    let mut zeroes = vec![[0; 5]];
    for i in 0..MERKLE_DEPTH {
        zeroes.push(pair(zeroes[i], zeroes[i]));
    }
    let leaves = (0..4)
        .map(|i| {
            let mut words = vec![DOMAIN_STAMP, public[0], public[1]];
            words.extend(&secret[19 + 18 * i..24 + 18 * i]);
            hash(&words)
        })
        .collect::<Vec<_>>();
    let mut levels = vec![leaves];
    for depth in 0..MERKLE_DEPTH {
        let previous = &levels[depth];
        let next = previous
            .chunks(2)
            .map(|p| pair(p[0], *p.get(1).unwrap_or(&zeroes[depth])))
            .collect::<Vec<_>>();
        levels.push(next);
    }
    public[67..72].copy_from_slice(&levels[MERKLE_DEPTH][0]);
    path.truncate(MERKLE_DEPTH * 4);
    for i in 0..4 {
        secret[91 + i] = i as u64;
        for depth in 0..MERKLE_DEPTH {
            path.push(
                *levels[depth]
                    .get((i >> depth) ^ 1)
                    .unwrap_or(&zeroes[depth]),
            );
        }
    }
}
#[test]
fn stamp_ownership_and_replay() {
    let secret = [11, 22, 33, 44, 55];
    let mut public = vec![0; OWNER_PUBLIC_WORDS];
    public[0] = 8979;
    public[2..7].copy_from_slice(&hash(&[DOMAIN_OWNER, 11, 22, 33, 44, 55]));
    public[7] = 1234;
    let proof = prove_owner(&public, &secret).expect("real stamp ownership proof");
    verify_owner(&public, &proof).unwrap();
    for index in [0, 2, 7, 22] {
        let mut changed = public.clone();
        changed[index] ^= 1;
        assert!(
            verify_owner(&changed, &proof).is_err(),
            "stamp proof replay {index}"
        );
    }
    assert!(prove_owner(&public, &[11, 22, 33, 44, 56]).is_err());
}

#[test]
fn unregistered_recipient_cannot_receive_even_with_valid_note_openings() {
    let (mut public, mut secret, path) = fixture();
    secret[19..24].copy_from_slice(&hash(&[DOMAIN_OWNER, 999, 888, 777, 666, 555]));
    refresh_commitments(&mut public, &secret, &path);
    // Conservation, input path, nullifier and every output opening are valid;
    // only stamp membership is false.
    assert!(!run(&public, &secret, &path));
    assert!(prove_spend(&public, &secret, &path).is_err());
}

fn multi_fixture(count: usize) -> (Vec<u64>, Vec<u64>, Vec<[u64; 5]>) {
    let (mut public, mut secret, mut paths) = fixture();
    public[87] = count as u64;
    secret[10] -= (count - 1) as u64;
    secret[18] = 0;
    let owner = hash(&[DOMAIN_OWNER, 11, 22, 33, 44, 55]);
    let mut leaves = vec![note(&public[..4], &owner, &secret[5..10], &secret[10..18])];
    let mut n = vec![DOMAIN_NULLIFIER];
    n.extend(&public[..4]);
    n.extend(hash(&[DOMAIN_NULLIFIER_KEY, 11, 22, 33, 44, 55]));
    n.extend(&secret[5..18]);
    public[33..38].copy_from_slice(&hash(&n));
    for i in 1..count {
        let base = 95 + (i - 1) * 14;
        for j in 0..5 {
            secret[base + j] = (800 + i * 10 + j) as u64;
        }
        secret[base + 5] = 1;
        secret[base + 13] = i as u64;
        leaves.push(note(
            &public[..4],
            &owner,
            &secret[base..base + 5],
            &secret[base + 5..base + 13],
        ));
        let mut n = vec![DOMAIN_NULLIFIER];
        n.extend(&public[..4]);
        n.extend(hash(&[DOMAIN_NULLIFIER_KEY, 11, 22, 33, 44, 55]));
        n.extend(&secret[base..base + 13]);
        public[72 + (i - 1) * 5..77 + (i - 1) * 5].copy_from_slice(&hash(&n));
    }
    let mut zeroes = vec![[0; 5]];
    for depth in 0..MERKLE_DEPTH {
        zeroes.push(pair(zeroes[depth], zeroes[depth]));
    }
    let mut levels = vec![leaves];
    for depth in 0..MERKLE_DEPTH {
        levels.push(
            levels[depth]
                .chunks(2)
                .map(|p| pair(p[0], *p.get(1).unwrap_or(&zeroes[depth])))
                .collect(),
        );
    }
    public[28..33].copy_from_slice(&levels[MERKLE_DEPTH][0]);
    for i in 0..count {
        for depth in 0..MERKLE_DEPTH {
            paths[i * MERKLE_DEPTH + depth] = *levels[depth]
                .get((i >> depth) ^ 1)
                .unwrap_or(&zeroes[depth]);
        }
    }
    (public, secret, paths)
}
#[test]
fn multiple_inputs_bind_each_opening_path_and_nullifier() {
    for count in 2..=4 {
        let (public, secret, path) = multi_fixture(count);
        assert!(run(&public, &secret, &path), "valid {count}-input spend");
        for i in 1..count {
            for offset in 0..14 {
                let mut bad = secret.clone();
                bad[95 + (i - 1) * 14 + offset] ^= 1;
                assert!(!run(&public, &bad, &path));
            }
            let mut bad = public.clone();
            let duplicate: Vec<_> = bad[33..38].to_vec();
            bad[72 + (i - 1) * 5..77 + (i - 1) * 5].copy_from_slice(&duplicate);
            assert!(!run(&bad, &secret, &path));
            let mut bad = path.clone();
            bad[i * MERKLE_DEPTH][0] ^= 1;
            assert!(!run(&public, &secret, &bad));
        }
        let mut bad = public.clone();
        bad[87] = 5;
        assert!(!run(&bad, &secret, &path));
    }
}
#[test]
fn real_four_input_stark_roundtrip() {
    let (public, secret, path) = multi_fixture(4);
    let proof = prove_spend(&public, &secret, &path).expect("four-input STARK within trace bound");
    verify_spend(&public, &proof).unwrap();
    let mut bad = public.clone();
    bad[72] ^= 1;
    assert!(verify_spend(&bad, &proof).is_err());
}

#[test]
fn sender_known_opening_does_not_identify_nullifier() {
    let (mut public, secret, path) = fixture();
    let mut old = vec![DOMAIN_NULLIFIER];
    old.extend(&public[..4]);
    old.extend(hash(&[DOMAIN_OWNER, 11, 22, 33, 44, 55]));
    old.extend(&secret[5..18]);
    assert_ne!(public[33..38], hash(&old));
    public[33..38].copy_from_slice(&hash(&old));
    assert!(!run(&public, &secret, &path));
}
#[test]
fn single_input_trace_omits_unused_paths() {
    let (public, secret, path) = fixture();
    let active = spending_paths(&public, &path).unwrap();
    assert_eq!(active.len(), MERKLE_DEPTH * 5);
    let tokens = canonical_words(&secret, SECRET_WORDS).unwrap();
    let digests: Vec<_> = active
        .iter()
        .map(|p| Digest::new(p.map(BFieldElement::new)))
        .collect();
    let (aet, _) = VM::trace_execution(
        spend_program(),
        PublicInput::new(canonical_words(&public, PUBLIC_WORDS).unwrap()),
        NonDeterminism::new(tokens).with_digests(digests),
    )
    .unwrap();
    eprintln!(
        "single-input padded trace: {} rows; tables: {:?}",
        aet.padded_height(),
        aet.height()
    );
    assert!(aet.padded_height() <= MAX_PADDED_HEIGHT);
}
