//! Shield3 private-spend relation and native Triton VM zk-STARK verifier.
//!
//! This is a separate V3 relation, not a verifier for legacy BN254 commitments.
//! Consensus must authenticate the supplied anchor and persist nullifiers.

use triton_vm::prelude::*;

pub const PUBLIC_WORDS: usize = 72;
pub const SECRET_WORDS: usize = 95;
pub const MERKLE_DEPTH: usize = 32;
pub const PATH_DIGESTS: usize = MERKLE_DEPTH * 5;
pub const OWNER_PUBLIC_WORDS: usize = 23;
pub const DOMAIN_STAMP: u64 = 3004;
pub const MAX_PROOF_WORDS: usize = 1 << 20;
pub const MAX_PADDED_HEIGHT: usize = 1 << 15;
pub const FIELD_MODULUS: u64 = 0xffff_ffff_0000_0001;
pub const DOMAIN_OWNER: u64 = 3001;
pub const DOMAIN_NOTE: u64 = 3002;
pub const DOMAIN_NULLIFIER: u64 = 3003;

// Public word order: chain lo/hi, asset lo/hi, public value (eight u32 limbs),
// transaction intent (sixteen u32 words), anchor (five field words),
// nullifier (five field words), four output commitments (five words each).
// Secret words: spending secret (5), note randomness (5), input value (8),
// leaf index (1), four outputs: owner digest (5), randomness (5), value (8).

#[derive(Clone, Copy)]
enum Word {
    Literal(u64),
    Memory(usize),
}

struct Assembly(String);
impl Assembly {
    fn emit(&mut self, text: &str) {
        self.0.push_str(text);
        self.0.push('\n');
    }
    fn load(&mut self, address: usize) {
        self.emit(&format!("push {address} read_mem 1 pop 1"));
    }
    fn digest(&mut self, address: usize) {
        self.emit(&format!("push {} read_mem 5 pop 1", address + 4));
    }
    fn store_digest(&mut self, address: usize) {
        self.emit(&format!("push {address} write_mem 5 pop 1"));
    }
    fn assert_digest(&mut self, address: usize) {
        self.digest(address);
        self.emit("assert_vector pop 5");
    }
    fn u32(&mut self, address: usize) {
        self.emit(&format!("push {address} call check_u32"));
    }
    fn hash(&mut self, words: Vec<Word>) {
        // Exactly Tip5::hash_varlen: append 1, then zero-pad to a rate block.
        let mut padded = words;
        padded.push(Word::Literal(1));
        while padded.len() % 10 != 0 {
            padded.push(Word::Literal(0));
        }
        self.emit("sponge_init");
        for block in padded.chunks(10) {
            let reversed = block.iter().rev().copied().collect::<Vec<_>>();
            let mut i = 0;
            while i < reversed.len() {
                match reversed[i] {
                    Word::Literal(v) => {
                        self.emit(&format!("push {v}"));
                        i += 1;
                    }
                    Word::Memory(end) => {
                        let mut count = 1;
                        while count < 5 && i + count < reversed.len() {
                            match reversed[i + count] {
                                Word::Memory(next) if next + count == end => count += 1,
                                _ => break,
                            }
                        }
                        self.emit(&format!("push {end} read_mem {count} pop 1"));
                        i += count;
                    }
                }
            }
            self.emit("sponge_absorb");
        }
        self.emit("sponge_squeeze");
        self.store_digest(300);
        self.emit("pop 5"); // discard the unused half of the squeeze
        self.digest(300);
    }
    fn note(&mut self, owner: usize, randomness: usize, value: usize) {
        let mut words = vec![Word::Literal(DOMAIN_NOTE)];
        words.extend((0..4).map(Word::Memory)); // chain and asset
        words.extend((owner..owner + 5).map(Word::Memory));
        words.extend((value..value + 8).map(Word::Memory));
        words.extend((randomness..randomness + 5).map(Word::Memory));
        self.hash(words);
    }
}

/// Frozen spending computation. The verifier derives its digest locally;
/// neither a prover-supplied program nor a prover-supplied claim is accepted.
pub fn spend_program() -> Program {
    let mut a = Assembly(String::new());
    for (length, base, instruction) in [(PUBLIC_WORDS, 0, "read_io"), (SECRET_WORDS, 100, "divine")]
    {
        for i in (0..length).step_by(5) {
            let count = (length - i).min(5);
            a.emit(&format!("{instruction} {count}"));
            // read_io/divine put the final token on top; memory stores pop
            // from the top. Reverse each batch to retain canonical ordering.
            for left in 0..count / 2 {
                let right = count - 1 - left;
                if left == 0 {
                    a.emit(&format!("swap {right}"));
                } else {
                    a.emit(&format!("swap {left} swap {right} swap {left}"));
                }
            }
            a.emit(&format!("push {} write_mem {count} pop 1", base + i));
        }
    }
    for i in 0..28 {
        a.u32(i);
    }
    for i in 110..119 {
        a.u32(i);
    }
    for i in 0..4 {
        for j in 0..8 {
            a.u32(129 + 18 * i + j);
        }
    }
    // Chain ID is nonzero. Input value is a nonzero 256-bit integer.
    a.load(0);
    a.load(1);
    a.emit("add push 0 eq push 0 eq assert");
    a.emit("push 1");
    for i in 110..118 {
        a.load(i);
        a.emit("push 0 eq mul");
    }
    a.emit("push 0 eq assert");
    // The final public word selects a spend (0) or a deposit (1).
    a.load(58);
    a.emit("dup 0 push 0 eq swap 1 push 1 eq add assert");
    let mut owner = vec![Word::Literal(DOMAIN_OWNER)];
    owner.extend((100..105).map(Word::Memory));
    a.hash(owner);
    a.store_digest(200);
    a.load(58);
    a.emit("push 0 eq skiz call private_spend");
    a.load(58);
    a.emit("push 1 eq skiz call public_deposit");
    // Slot 3 is change. Its owner must be the input spending secret's owner.
    // A recipient cannot hide an over-limit payment in this reserved slot.
    a.digest(173);
    a.assert_digest(200);
    for i in 0..4 {
        a.note(119 + 18 * i, 124 + 18 * i, 129 + 18 * i);
        a.assert_digest(38 + 5 * i);
    }

    // Every output owner, including change and decoys, belongs to the
    // immutable, consensus-authenticated stamp registry. Membership witnesses
    // stay private; output owners and registration indices are not public.
    for i in 0..4 {
        a.u32(191 + i);
        let mut stamp = vec![
            Word::Literal(DOMAIN_STAMP),
            Word::Memory(0),
            Word::Memory(1),
        ];
        stamp.extend((119 + 18 * i..124 + 18 * i).map(Word::Memory));
        a.hash(stamp);
        a.store_digest(310);
        a.load(191 + i);
        a.digest(310);
        for _ in 0..MERKLE_DEPTH {
            a.emit("merkle_step");
        }
        a.assert_digest(67);
        a.emit("push 0 eq assert");
    }

    // Public sponsorship pays fees and is excluded from the send amount.
    // Checked subtraction forbids sponsorship exceeding the public release.
    a.emit("push 0");
    for limb in 0..8 {
        a.u32(59 + limb);
        a.emit("push -1 mul");
        a.load(4 + limb);
        a.emit("add");
        a.load(59 + limb);
        a.emit("push -1 mul add push 4294967296 add split");
        a.emit(&format!("push {} write_mem 1 pop 1", 420 + limb));
        a.emit("push -1 mul push 1 add");
    }
    a.emit("push 0 eq assert");
    // Sum withdrawal and the three payment slots as a full 256-bit integer.
    a.emit("push 0");
    for limb in 0..8 {
        a.load(420 + limb);
        a.emit("add");
        for i in 0..3 {
            a.load(129 + 18 * i + limb);
            a.emit("add");
        }
        a.emit("split");
        a.emit(&format!("push {} write_mem 1 pop 1", 400 + limb));
    }
    a.emit("push 0 eq assert");
    // cap - sum, with a checked borrow per limb. Final borrow must be zero.
    a.emit("push 0");
    a.emit("push -1 mul push 620756992 add");
    a.load(400);
    a.emit("push -1 mul add push 4294967296 add split pop 1 push -1 mul push 1 add");
    a.emit("push -1 mul push 2332688548 add");
    a.load(401);
    a.emit("push -1 mul add push 4294967296 add split pop 1 push -1 mul push 1 add");
    a.emit("push -1 mul push 271050 add");
    a.load(402);
    a.emit("push -1 mul add push 4294967296 add split pop 1 push -1 mul push 1 add");
    a.emit("push -1 mul push 0 add");
    a.load(403);
    a.emit("push -1 mul add push 4294967296 add split pop 1 push -1 mul push 1 add");
    a.emit("push -1 mul push 0 add");
    a.load(404);
    a.emit("push -1 mul add push 4294967296 add split pop 1 push -1 mul push 1 add");
    a.emit("push -1 mul push 0 add");
    a.load(405);
    a.emit("push -1 mul add push 4294967296 add split pop 1 push -1 mul push 1 add");
    a.emit("push -1 mul push 0 add");
    a.load(406);
    a.emit("push -1 mul add push 4294967296 add split pop 1 push -1 mul push 1 add");
    a.emit("push -1 mul push 0 add");
    a.load(407);
    a.emit("push -1 mul add push 4294967296 add split pop 1 push -1 mul push 1 add");
    a.emit("push 0 eq assert");

    // Exact 256-bit integer conservation. Every limb is range checked.
    // The sum of five u32 limbs and carry is < 2^35, so no field wrap is
    // possible. Splitting propagates the carry; a final carry is forbidden.
    a.emit("push 0");
    for limb in 0..8 {
        a.load(4 + limb);
        a.emit("add");
        for i in 0..4 {
            a.load(129 + 18 * i + limb);
            a.emit("add");
        }
        a.emit("split");
        a.load(110 + limb);
        a.emit("eq assert");
    }
    a.emit("push 0 eq assert halt");
    a.emit("private_spend:");
    a.emit("push 1");
    for i in 33..38 {
        a.load(i);
        a.emit("push 0 eq mul");
    }
    a.emit("push 0 eq assert");
    a.note(200, 105, 110);
    a.store_digest(205);
    a.load(118);
    a.digest(205);
    for _ in 0..MERKLE_DEPTH {
        a.emit("merkle_step");
    }
    a.assert_digest(28);
    a.emit("push 0 eq assert");
    let mut nullifier = vec![Word::Literal(DOMAIN_NULLIFIER)];
    nullifier.extend((0..4).map(Word::Memory));
    nullifier.extend((200..205).map(Word::Memory));
    nullifier.extend((105..118).map(Word::Memory));
    a.hash(nullifier);
    a.assert_digest(33);
    a.emit("return");
    a.emit("public_deposit:");
    // Consume the fixed note-path prefix also for deposits, so stamp paths
    // have the same position in all witness encodings.
    a.emit("push 0 push 0 push 0 push 0 push 0 push 0");
    for _ in 0..MERKLE_DEPTH {
        a.emit("merkle_step");
    }
    a.emit("pop 5 pop 1");
    for i in 28..38 {
        a.load(i);
        a.emit("push 0 eq assert");
    }
    for limb in 0..8 {
        a.load(4 + limb);
        a.load(110 + limb);
        a.emit("eq assert");
        // Deposits use public value as the input, not as a public release.
        a.emit(&format!("push 0 push {} write_mem 1 pop 1", 4 + limb));
    }
    a.emit("return");
    a.emit("check_u32: read_mem 1 pop 1 split pop 1 push 0 eq assert return");
    Program::from_code(&a.0).expect("valid, fixed Shield3 assembly")
}

pub fn canonical_words(words: &[u64], size: usize) -> Result<Vec<BFieldElement>, String> {
    if words.len() != size || words.iter().any(|&v| v >= FIELD_MODULUS) {
        return Err("invalid field-element encoding".into());
    }
    Ok(words.iter().copied().map(BFieldElement::new).collect())
}

fn public_input(words: &[u64]) -> Result<Vec<BFieldElement>, String> {
    let result = canonical_words(words, PUBLIC_WORDS)?;
    if (words[..28].iter().chain(words[59..67].iter())).any(|&v| v > u32::MAX as u64)
        || (words[0] == 0 && words[1] == 0)
    {
        return Err("invalid public input range".into());
    }
    if words[67..72].iter().all(|&v| v == 0) {
        return Err("zero stamp root".into());
    }
    if words[58] > 1 {
        return Err("invalid proof kind".into());
    }
    if words[58] == 1 && words[28..38].iter().any(|&v| v != 0) {
        return Err("deposit has private state references".into());
    }
    if words[58] == 0 && words[28..33].iter().all(|&v| v == 0) {
        return Err("zero anchor".into());
    }
    if words[58] == 0 && words[33..38].iter().all(|&v| v == 0) {
        return Err("zero nullifier".into());
    }
    Ok(result)
}

pub fn prove_spend(public: &[u64], secret: &[u64], path: &[[u64; 5]]) -> Result<Vec<u64>, String> {
    let public = public_input(public)?;
    let tokens = canonical_words(secret, SECRET_WORDS)?;
    if path.len() != PATH_DIGESTS {
        return Err("invalid Merkle path depth".into());
    }
    let digests = path
        .iter()
        .map(|d| canonical_words(d, 5).map(|words| Digest::new(words.try_into().unwrap())))
        .collect::<Result<Vec<_>, _>>()?;
    prove_program(spend_program(), public, tokens, digests)
}

fn prove_program(
    program: Program,
    public: Vec<BFieldElement>,
    tokens: Vec<BFieldElement>,
    digests: Vec<Digest>,
) -> Result<Vec<u64>, String> {
    let nondeterminism = NonDeterminism::new(tokens).with_digests(digests);
    let (aet, output) = VM::trace_execution(
        program.clone(),
        PublicInput::new(public.clone()),
        nondeterminism,
    )
    .map_err(|_| "invalid private spending witness".to_string())?;
    if !output.is_empty() || aet.padded_height() > MAX_PADDED_HEIGHT {
        return Err("spending trace exceeds fixed limits".into());
    }
    let claim = Claim::about_program(&program).with_input(public);
    let proof = Stark::default()
        .prove(&claim, &aet)
        .map_err(|e| format!("STARK proving failed: {e}"))?;
    if proof.0.len() > MAX_PROOF_WORDS {
        return Err("STARK proof exceeds fixed limits".into());
    }
    Ok(proof.0.iter().map(|v| v.value()).collect())
}

pub fn verify_spend(public: &[u64], words: &[u64]) -> Result<(), String> {
    verify_program(spend_program(), public_input(public)?, words)
}

fn verify_program(
    program: Program,
    public: Vec<BFieldElement>,
    words: &[u64],
) -> Result<(), String> {
    if words.is_empty() || words.len() > MAX_PROOF_WORDS {
        return Err("invalid proof size".into());
    }
    let proof = Proof(canonical_words(words, words.len())?);
    let claim = Claim::about_program(&program).with_input(public);
    // The upstream parser/verifier can panic on hostile streams. Such input
    // must fail closed rather than killing a caller or accepting a proof.
    let result = std::panic::catch_unwind(|| {
        // Bound the exponent before shifting. Release builds must not wrap
        // malicious exponents such as 64 or u32::MAX into a small height.
        let stream = triton_vm::proof_stream::ProofStream::try_from(&proof)
            .map_err(|_| "malformed proof trace")?;
        let mut heights = stream
            .items
            .into_iter()
            .filter_map(|item| item.try_into_log2_padded_height().ok());
        let exponent = heights.next().ok_or("missing proof trace height")?;
        if heights.next().is_some() || exponent > MAX_PADDED_HEIGHT.ilog2() {
            return Err("proof trace exceeds fixed limits");
        }
        Stark::default()
            .verify(&claim, &proof)
            .map_err(|_| "invalid STARK spend proof")
    });
    result
        .map_err(|_| "malformed STARK proof".to_string())?
        .map_err(str::to_string)
}

/// Compute wallet note openings using the same specified Tip5 domains. The
/// returned root is only a witness-derived candidate: consensus must compare
/// it to its own canonical tree. Nothing is verified by this helper.
/// Output: input owner, input commitment, root, nullifier, four outputs (5 each).
pub fn describe_spend(
    public: &[u64],
    secret: &[u64],
    path: &[[u64; 5]],
) -> Result<Vec<u64>, String> {
    canonical_words(public, PUBLIC_WORDS)?;
    canonical_words(secret, SECRET_WORDS)?;
    if public[..4].iter().any(|&v| v > u32::MAX as u64) || (public[0] == 0 && public[1] == 0) {
        return Err("invalid chain or asset".into());
    }
    if path.len() != PATH_DIGESTS || secret[10..19].iter().any(|&v| v > u32::MAX as u64) {
        return Err("invalid private input range".into());
    }
    for i in 0..4 {
        if secret[29 + 18 * i..37 + 18 * i]
            .iter()
            .any(|&v| v > u32::MAX as u64)
        {
            return Err("invalid output amount range".into());
        }
    }
    let hash = |words: &[u64]| {
        Tip5::hash_varlen(
            &words
                .iter()
                .copied()
                .map(BFieldElement::new)
                .collect::<Vec<_>>(),
        )
    };
    let mut owner_words = vec![DOMAIN_OWNER];
    owner_words.extend(&secret[..5]);
    let owner = hash(&owner_words);
    let note = |owner: &[u64], random: &[u64], value: &[u64]| {
        let mut words = vec![DOMAIN_NOTE];
        words.extend(&public[..4]);
        words.extend(owner);
        words.extend(value);
        words.extend(random);
        hash(&words)
    };
    let owner_words = owner.values().map(|v| v.value());
    let input = note(&owner_words, &secret[5..10], &secret[10..18]);
    let mut root = input;
    let mut index = secret[18];
    for sibling in path.iter().take(MERKLE_DEPTH) {
        let sibling = Digest::new(canonical_words(sibling, 5)?.try_into().unwrap());
        root = if index & 1 == 0 {
            Tip5::hash_pair(root, sibling)
        } else {
            Tip5::hash_pair(sibling, root)
        };
        index >>= 1;
    }
    let mut null_words = vec![DOMAIN_NULLIFIER];
    null_words.extend(&public[..4]);
    null_words.extend(&owner_words);
    null_words.extend(&secret[5..18]);
    let nullifier = hash(&null_words);
    let mut result = Vec::with_capacity(40);
    for digest in [owner, input, root, nullifier] {
        result.extend(digest.values().map(|v| v.value()));
    }
    for i in 0..4 {
        let base = 19 + 18 * i;
        result.extend(
            note(
                &secret[base..base + 5],
                &secret[base + 5..base + 10],
                &secret[base + 10..base + 18],
            )
            .values()
            .map(|v| v.value()),
        );
    }
    Ok(result)
}

#[cfg(test)]
mod tests;

mod ffi;

// Registration proves knowledge of the private owner preimage and binds the
// complete unsigned registration intent. An observer cannot register somebody
// else's publicly visible owner digest with a copied proof.
fn owner_program() -> Program {
    let mut a = Assembly(String::new());
    for (length, base, instruction) in [(OWNER_PUBLIC_WORDS, 0, "read_io"), (5, 100, "divine")] {
        for i in 0..length {
            a.emit(&format!(
                "{instruction} 1 push {} write_mem 1 pop 1",
                base + i
            ));
        }
    }
    for i in [0, 1].into_iter().chain(7..23) {
        a.u32(i);
    }
    a.load(0);
    a.load(1);
    a.emit("add push 0 eq push 0 eq assert");
    let mut owner = vec![Word::Literal(DOMAIN_OWNER)];
    owner.extend((100..105).map(Word::Memory));
    a.hash(owner);
    a.assert_digest(2);
    a.emit("halt");
    a.emit("check_u32: read_mem 1 pop 1 split pop 1 push 0 eq assert return");
    Program::from_code(&a.0).expect("valid fixed stamp ownership program")
}
fn owner_input(public: &[u64]) -> Result<Vec<BFieldElement>, String> {
    let words = canonical_words(public, OWNER_PUBLIC_WORDS)?;
    if public[..2]
        .iter()
        .chain(public[7..].iter())
        .any(|&v| v > u32::MAX as u64)
        || (public[0] == 0 && public[1] == 0)
        || public[2..7].iter().all(|&v| v == 0)
    {
        return Err("invalid stamp ownership statement".into());
    }
    Ok(words)
}
pub fn prove_owner(public: &[u64], secret: &[u64]) -> Result<Vec<u64>, String> {
    prove_program(
        owner_program(),
        owner_input(public)?,
        canonical_words(secret, 5)?,
        vec![],
    )
}
pub fn verify_owner(public: &[u64], proof: &[u64]) -> Result<(), String> {
    verify_program(owner_program(), owner_input(public)?, proof)
}
