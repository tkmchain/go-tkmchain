//! Shield3 private-spend relation and native Triton VM zk-STARK verifier.
//!
//! This is a separate V3 relation, not a verifier for legacy BN254 commitments.
//! Consensus must authenticate the supplied anchor and persist nullifiers.

use triton_vm::prelude::*;

pub const PUBLIC_WORDS: usize = 108;
pub const PUBLIC_WORDS_V4: usize = 113;
pub const SECRET_WORDS: usize = 137;
pub const MERKLE_DEPTH: usize = 32;
pub const PATH_DIGESTS: usize = MERKLE_DEPTH * 8;
pub const OWNER_PUBLIC_WORDS: usize = 23;
pub const DOMAIN_STAMP: u64 = 3004;
pub const MAX_PROOF_WORDS: usize = 1 << 20;
pub const MAX_PADDED_HEIGHT: usize = 1 << 16;
pub const FIELD_MODULUS: u64 = 0xffff_ffff_0000_0001;
pub const DOMAIN_OWNER: u64 = 3001;
pub const DOMAIN_NOTE: u64 = 3002;
pub const DOMAIN_NULLIFIER: u64 = 3003;
pub const DOMAIN_NULLIFIER_KEY: u64 = 3005;
pub const DOMAIN_ONETIME_KEY: u64 = 3006;
pub const DOMAIN_SHIELD4_PROGRAM: u64 = 4004;
pub const DOMAIN_SHIELD4_TAG: u64 = 4007;
pub const DOMAIN_PRIVATE_TVM_LEAF: u64 = 5001;
pub const PRIVATE_TVM_PUBLIC_WORDS: usize = 34;
pub const PRIVATE_TVM_SECRET_WORDS: usize = 21;
pub const PRIVATE_TVM_PATH_DIGESTS: usize = MERKLE_DEPTH;

// Public word order: chain lo/hi, asset lo/hi, public value (eight u32 limbs),
// transaction intent (sixteen u32 words), anchor (five field words),
// nullifier (five field words), four output commitments (five words each),
// four proved one-time output keys (five words each).
// Secret words: spending secret (5), note randomness (5), input value (8),
// leaf index (1), four outputs: owner digest (5), randomness (5), value (8).

#[derive(Clone, Copy)]
enum Word {
    Literal(u64),
    Memory(usize),
    Pointer(usize, usize),
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
                    Word::Pointer(pointer, end) => {
                        let mut count = 1;
                        while count < 5 && i + count < reversed.len() {
                            match reversed[i + count] {
                                Word::Pointer(next_pointer, next)
                                    if next_pointer == pointer && next + count == end =>
                                {
                                    count += 1
                                }
                                _ => break,
                            }
                        }
                        self.emit(&format!(
                            "push {pointer} read_mem 1 pop 1 push {end} add read_mem {count} pop 1"
                        ));
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
        self.emit(&format!(
            "push {owner} push {randomness} push {value} call note_hash"
        ));
    }
    fn note_routine(&mut self) {
        self.emit("note_hash: push 402 write_mem 1 pop 1 push 401 write_mem 1 pop 1 push 400 write_mem 1 pop 1");
        let mut words = vec![Word::Literal(DOMAIN_NOTE)];
        words.extend((0..4).map(Word::Memory));
        words.extend((0..5).map(|offset| Word::Pointer(400, offset)));
        words.extend((0..8).map(|offset| Word::Pointer(402, offset)));
        words.extend((0..5).map(|offset| Word::Pointer(401, offset)));
        self.hash(words);
        self.emit("return");
    }
}

/// Frozen spending computation. The verifier derives its digest locally;
/// neither a prover-supplied program nor a prover-supplied claim is accepted.
fn spend_program_with_domain(domain: Option<u64>, shield4: bool) -> Program {
    let mut a = Assembly(String::new());
    // Shield4 uses the same audited witness relation but a distinct frozen
    // claim program.  This constant assertion changes the program digest and
    // prevents a proof produced for one protocol version from verifying under
    // the other version, even when the public inputs happen to match.
    if let Some(domain) = domain {
        a.emit(&format!("push {domain} push {domain} eq assert"));
    }
    let public_words = if shield4 {
        PUBLIC_WORDS_V4
    } else {
        PUBLIC_WORDS
    };
    for (length, base, instruction) in
        [(public_words, 0, "read_io"), (SECRET_WORDS, 1000, "divine")]
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
    a.emit("push 0 push 28 call check_range");
    a.emit("push 1010 push 9 call check_range");
    for i in 0..4 {
        a.emit(&format!("push {} push 8 call check_range", 1029 + 18 * i));
    }
    // Chain ID is nonzero. Input value is a nonzero 256-bit integer.
    a.load(0);
    a.load(1);
    a.emit("add push 0 eq push 0 eq assert");
    a.emit("push 1");
    for i in 1010..1018 {
        a.load(i);
        a.emit("push 0 eq mul");
    }
    a.emit("push 0 eq assert");
    // The final public word selects a spend (0) or a deposit (1).
    a.load(58);
    a.emit("dup 0 push 0 eq swap 1 push 1 eq add assert");
    a.u32(87);
    a.load(87);
    a.emit("push 0 eq");
    a.load(58);
    a.emit("eq assert");
    a.emit("push 0");
    for count in 0..=4 {
        a.load(87);
        a.emit(&format!("push {count} eq add"));
    }
    a.emit("push 1 eq assert");
    // Aggregate input value starts with the first note (or deposit amount).
    for limb in 0..8 {
        a.load(1010 + limb);
        a.emit(&format!("push {} write_mem 1 pop 1", 500 + limb));
    }
    let mut owner = vec![Word::Literal(DOMAIN_OWNER)];
    owner.extend((1000..1005).map(Word::Memory));
    a.hash(owner);
    a.store_digest(200);
    // Bind the key to the same owner preimage; arbitrary keys enable double spends.
    let mut key = vec![Word::Literal(DOMAIN_NULLIFIER_KEY)];
    key.extend((1000..1005).map(Word::Memory));
    a.hash(key);
    a.store_digest(210);
    a.load(58);
    a.emit("push 0 eq skiz call private_spend");
    a.load(58);
    a.emit("push 1 eq skiz call public_deposit");
    // Three additional notes share the same hidden spending owner. The public
    // input count determines which openings must prove membership and a unique
    // nullifier; inactive slots are zero and consume no VM digest paths.
    for i in 0..3 {
        a.emit("push 0");
        for count in i + 2..=4 {
            a.load(87);
            a.emit(&format!("push {count} eq add"));
        }
        a.emit(&format!(
            "dup 0 skiz call extra_spend_{i} push 0 eq skiz call inactive_input_{i}"
        ));
    }
    // Slot 3 is change. Its owner must be the input spending secret's owner.
    // A recipient cannot hide an over-limit payment in this reserved slot.
    a.digest(1073);
    a.assert_digest(200);
    for i in 0..4 {
        let base = 1019 + 18 * i;
        a.note(base, base + 5, base + 10);
        a.assert_digest(38 + 5 * i);
        // The public one-time key is a Tip5 derivation of the hidden output
        // opening and the proved commitment. This prevents metadata
        // substitution: the key cannot be changed without invalidating the
        // STARK proof.
        let mut one_time = vec![Word::Literal(DOMAIN_ONETIME_KEY)];
        one_time.extend((0..5).map(|offset| Word::Memory(base + offset)));
        one_time.extend((0..5).map(|offset| Word::Memory(base + 5 + offset)));
        one_time.extend((0..5).map(|offset| Word::Memory(300 + offset)));
        a.hash(one_time);
        a.store_digest(320);
        a.digest(320);
        a.assert_digest(88 + 5 * i);
    }

    // Every output owner, including change and decoys, belongs to the
    // immutable, consensus-authenticated stamp registry. Membership witnesses
    // stay private; output owners and registration indices are not public.
    for i in 0..4 {
        a.u32(1091 + i);
        a.emit(&format!("push {} call stamp_hash", 1019 + 18 * i));
        a.store_digest(310);
        a.load(1091 + i);
        a.digest(310);
        a.emit("call merkle_path");
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
            a.load(1029 + 18 * i + limb);
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
            a.load(1029 + 18 * i + limb);
            a.emit("add");
        }
        a.emit("split");
        a.load(500 + limb);
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
    a.note(200, 1005, 1010);
    a.store_digest(205);
    a.load(1018);
    a.digest(205);
    a.emit("call merkle_path");
    a.assert_digest(28);
    a.emit("push 0 eq assert");
    a.emit("push 1005 call nullifier_hash");
    a.assert_digest(33);
    if shield4 {
        // Shield4 adds a deterministic, hidden-owner linkability tag to the
        // public statement. It is separate from the shared nullifier so the
        // state transition can enforce both version binding and one-spend
        // semantics without exposing the note commitment.
        let mut tag = vec![Word::Literal(DOMAIN_SHIELD4_TAG)];
        tag.extend((200..205).map(Word::Memory));
        tag.extend((1005..1010).map(Word::Memory));
        tag.push(Word::Memory(1018));
        tag.extend((28..33).map(Word::Memory));
        a.hash(tag);
        a.assert_digest(108);
    }
    a.emit("return");
    a.emit("public_deposit:");
    for i in 28..38 {
        a.load(i);
        a.emit("push 0 eq assert");
    }
    for limb in 0..8 {
        a.load(4 + limb);
        a.load(1010 + limb);
        a.emit("eq assert");
        // Deposits use public value as the input, not as a public release.
        a.emit(&format!("push 0 push {} write_mem 1 pop 1", 4 + limb));
    }
    a.emit("return");
    for i in 0..3 {
        let base = 1095 + 14 * i;
        let nullifier = 72 + 5 * i;
        a.emit(&format!("extra_spend_{i}:"));
        a.emit(&format!("push {} push 9 call check_range", base + 5));
        a.emit("push 1");
        for j in 5..13 {
            a.load(base + j);
            a.emit("push 0 eq mul");
        }
        a.emit("push 0 eq assert");
        a.note(200, base, base + 5);
        a.store_digest(205);
        a.load(base + 13);
        a.digest(205);
        a.emit("call merkle_path");
        a.assert_digest(28);
        a.emit("push 0 eq assert");
        a.emit(&format!("push {base} call nullifier_hash"));
        a.assert_digest(nullifier);
        // No repeated input, even when a malicious witness opens it twice.
        for previous in std::iter::once(33).chain((0..i).map(|j| 72 + 5 * j)) {
            a.emit("push 1");
            for j in 0..5 {
                a.load(previous + j);
                a.load(nullifier + j);
                a.emit("eq mul");
            }
            a.emit("push 0 eq assert");
        }
        a.emit("push 0");
        for limb in 0..8 {
            a.load(500 + limb);
            a.emit("add");
            a.load(base + 5 + limb);
            a.emit("add split");
            a.emit(&format!("push {} write_mem 1 pop 1", 500 + limb));
        }
        a.emit("push 0 eq assert return");
        a.emit(&format!("inactive_input_{i}:"));
        for j in 0..14 {
            a.load(base + j);
            a.emit("push 0 eq assert");
        }
        for j in 0..5 {
            a.load(nullifier + j);
            a.emit("push 0 eq assert");
        }
        a.emit("return");
    }
    a.emit("merkle_path: push 32 push 320 write_mem 1 pop 1 call merkle_loop return");
    a.emit("merkle_loop: merkle_step push 320 read_mem 1 pop 1 push -1 add dup 0 push 320 write_mem 1 pop 1 push 0 eq skiz return recurse");
    a.emit(
        "check_range: push 330 write_mem 1 pop 1 push 331 write_mem 1 pop 1 call range_loop return",
    );
    a.emit("range_loop: push 331 read_mem 1 pop 1 dup 0 call check_u32 push 1 add push 331 write_mem 1 pop 1 push 330 read_mem 1 pop 1 push -1 add dup 0 push 330 write_mem 1 pop 1 push 0 eq skiz return recurse");
    a.emit("nullifier_hash: push 400 write_mem 1 pop 1");
    let mut nullifier = vec![Word::Literal(DOMAIN_NULLIFIER)];
    nullifier.extend((0..4).map(Word::Memory));
    nullifier.extend((210..215).map(Word::Memory));
    nullifier.extend((0..13).map(|offset| Word::Pointer(400, offset)));
    a.hash(nullifier);
    a.emit("return");
    a.emit("stamp_hash: push 400 write_mem 1 pop 1");
    let mut stamp = vec![
        Word::Literal(DOMAIN_STAMP),
        Word::Memory(0),
        Word::Memory(1),
    ];
    stamp.extend((0..5).map(|offset| Word::Pointer(400, offset)));
    a.hash(stamp);
    a.emit("return");
    a.note_routine();
    a.emit("check_u32: read_mem 1 pop 1 split pop 1 push 0 eq assert return");
    Program::from_code(&a.0).expect("valid, fixed Shield assembly")
}

pub fn spend_program() -> Program {
    spend_program_with_domain(None, false)
}

/// Shield4 is a separate frozen claim program.  It keeps the same hidden
/// full-chain membership witness shape so existing notes remain spendable,
/// while the domain assertion gives the verifier a protocol-specific program
/// digest and therefore a non-interchangeable proof system.
pub fn spend_program_v4() -> Program {
    spend_program_with_domain(Some(DOMAIN_SHIELD4_PROGRAM), true)
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
    let count = words[87] as usize;
    if count > 4 || (words[58] == 1) != (count == 0) {
        return Err("invalid input count".into());
    }
    let mut active = Vec::new();
    for i in 0..4 {
        let base = if i == 0 { 33 } else { 72 + 5 * (i - 1) };
        let nullifier = &words[base..base + 5];
        if i < count {
            if nullifier.iter().all(|&v| v == 0) || active.contains(&nullifier) {
                return Err("zero or duplicate nullifier".into());
            }
            active.push(nullifier);
        } else if nullifier.iter().any(|&v| v != 0) {
            return Err("inactive input nullifier".into());
        }
    }
    Ok(result)
}

pub fn prove_spend(public: &[u64], secret: &[u64], path: &[[u64; 5]]) -> Result<Vec<u64>, String> {
    let active_path = spending_paths(public, path)?;
    let public = public_input(public)?;
    let tokens = canonical_words(secret, SECRET_WORDS)?;
    let digests = active_path
        .iter()
        .map(|d| canonical_words(d, 5).map(|words| Digest::new(words.try_into().unwrap())))
        .collect::<Result<Vec<_>, _>>()?;
    prove_program(spend_program(), public, tokens, digests)
}

// Keep the bounded helper ABI but omit unused note paths from the VM. Zero
// inactive encodings cannot shift active paths or output stamp witnesses.
fn spending_paths(public: &[u64], path: &[[u64; 5]]) -> Result<Vec<[u64; 5]>, String> {
    public_input(public)?;
    if path.len() != PATH_DIGESTS {
        return Err("invalid Merkle path depth".into());
    }
    let end = public[87] as usize * MERKLE_DEPTH;
    if path[end..4 * MERKLE_DEPTH]
        .iter()
        .flatten()
        .any(|&v| v != 0)
    {
        return Err("nonzero inactive input path".into());
    }
    let mut active = path[..end].to_vec();
    active.extend_from_slice(&path[4 * MERKLE_DEPTH..]);
    Ok(active)
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

pub fn prove_spend_v4(
    public: &[u64],
    secret: &[u64],
    path: &[[u64; 5]],
) -> Result<Vec<u64>, String> {
    let active_path = spending_paths_v4(public, path)?;
    let public = public_input_v4(public)?;
    let tokens = canonical_words(secret, SECRET_WORDS)?;
    let digests = active_path
        .iter()
        .map(|d| canonical_words(d, 5).map(|words| Digest::new(words.try_into().unwrap())))
        .collect::<Result<Vec<_>, _>>()?;
    prove_program(spend_program_v4(), public, tokens, digests)
}

pub fn verify_spend_v4(public: &[u64], words: &[u64]) -> Result<(), String> {
    verify_program(spend_program_v4(), public_input_v4(public)?, words)
}

/// Frozen private TVM storage transition relation.
///
/// Public words are: chain id (2), TVM code hash (5), old root (5), new root
/// (5), transaction intent (16), and operation (1; zero is a read and one is a
/// write). Secret words are the code hash (5), storage key (5), old value (5),
/// new value (5), and leaf index (1), plus one 32-level Merkle path. The same
/// path is supplied twice as nondeterminism so the relation proves both roots
/// against one authenticated state tree.
fn private_tvm_program() -> Program {
    let mut a = Assembly(String::new());
    for (length, base, instruction) in [
        (PRIVATE_TVM_PUBLIC_WORDS, 0, "read_io"),
        (PRIVATE_TVM_SECRET_WORDS, 1000, "divine"),
    ] {
        for i in (0..length).step_by(5) {
            let count = (length - i).min(5);
            a.emit(&format!("{instruction} {count}"));
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

    // The chain id, code hash, and intent are domain-separated public inputs.
    a.load(0);
    a.load(1);
    a.emit("add push 0 eq push 0 eq assert");
    a.emit("push 1");
    for i in 2..7 {
        a.load(i);
        a.emit("push 0 eq mul");
    }
    a.emit("push 0 eq assert");
    a.emit("push 1");
    for i in 17..33 {
        a.load(i);
        a.emit("push 0 eq mul");
    }
    a.emit("push 0 eq assert");
    a.load(33);
    a.emit("dup 0 push 0 eq swap 1 push 1 eq add assert");
    a.u32(1020);

    // The hidden contract identity must match the public code hash. The
    // storage tree itself is keyed by the hidden storage key, so the empty
    // tree root is independent of which contract is accessed.
    for i in 0..5 {
        a.load(1000 + i);
        a.load(2 + i);
        a.emit("eq assert");
    }

    // Old value -> old root.
    let mut old_leaf = vec![Word::Literal(DOMAIN_PRIVATE_TVM_LEAF)];
    old_leaf.extend((2..7).map(Word::Memory));
    old_leaf.extend((1005..1015).map(Word::Memory));
    a.hash(old_leaf);
    a.store_digest(300);
    a.load(1020);
    a.digest(300);
    a.emit("call merkle_path");
    a.assert_digest(7);
    a.emit("push 0 eq assert");

    // New value -> new root using the exact same hidden key and Merkle path.
    let mut new_leaf = vec![Word::Literal(DOMAIN_PRIVATE_TVM_LEAF)];
    new_leaf.extend((2..7).map(Word::Memory));
    new_leaf.extend((1005..1010).map(Word::Memory));
    new_leaf.extend((1015..1020).map(Word::Memory));
    a.hash(new_leaf);
    a.store_digest(300);
    a.load(1020);
    a.digest(300);
    a.emit("call merkle_path");
    a.assert_digest(12);
    a.emit("push 0 eq assert");

    // A read cannot change the committed value. A write may change it, but
    // the new root remains fully bound to the hidden key and new value.
    a.load(33);
    a.emit("push 0 eq skiz call private_tvm_read");
    a.load(33);
    a.emit("push 1 eq skiz call private_tvm_write");
    a.emit("halt");
    a.emit("private_tvm_read:");
    a.emit("push 1");
    for i in 0..5 {
        a.load(1010 + i);
        a.load(1015 + i);
        a.emit("eq mul");
    }
    a.emit("push 0 eq assert return");
    a.emit("private_tvm_write:");
    let mut expected = vec![Word::Literal(DOMAIN_PRIVATE_TVM_LEAF + 1)];
    expected.extend((2..7).map(Word::Memory));
    expected.extend((1005..1010).map(Word::Memory));
    expected.extend((1010..1015).map(Word::Memory));
    expected.extend((17..33).map(Word::Memory));
    a.hash(expected);
    a.store_digest(350);
    for i in 0..5 {
        a.load(1015 + i);
        a.load(350 + i);
        a.emit("eq assert");
    }
    a.emit("return");
    a.emit("merkle_path: push 32 push 320 write_mem 1 pop 1 call merkle_loop return");
    a.emit("merkle_loop: merkle_step push 320 read_mem 1 pop 1 push -1 add dup 0 push 320 write_mem 1 pop 1 push 0 eq skiz return recurse");
    a.emit("check_u32: read_mem 1 pop 1 split pop 1 push 0 eq assert return");
    Program::from_code(&a.0).expect("valid, fixed private TVM assembly")
}

fn private_tvm_input(words: &[u64]) -> Result<Vec<BFieldElement>, String> {
    if words.len() != PRIVATE_TVM_PUBLIC_WORDS {
        return Err("invalid private TVM public input length".into());
    }
    if words[0] == 0 && words[1] == 0 {
        return Err("private TVM chain id is zero".into());
    }
    if words[2..7].iter().all(|&v| v == 0) {
        return Err("private TVM code hash is zero".into());
    }
    if words[17..33].iter().all(|&v| v == 0) {
        return Err("private TVM intent is zero".into());
    }
    if words[33] > 1 {
        return Err("invalid private TVM operation".into());
    }
    canonical_words(words, PRIVATE_TVM_PUBLIC_WORDS)
}

pub fn prove_private_tvm(
    public: &[u64],
    secret: &[u64],
    path: &[[u64; 5]],
) -> Result<Vec<u64>, String> {
    let public = private_tvm_input(public)?;
    if secret.len() != PRIVATE_TVM_SECRET_WORDS || path.len() != PRIVATE_TVM_PATH_DIGESTS {
        return Err("invalid private TVM witness length".into());
    }
    let tokens = canonical_words(secret, PRIVATE_TVM_SECRET_WORDS)?;
    let path = path
        .iter()
        .map(|d| canonical_words(d, 5).map(|w| Digest::new(w.try_into().unwrap())))
        .collect::<Result<Vec<_>, _>>()?;
    let digests = path
        .iter()
        .cloned()
        .chain(path.iter().cloned())
        .collect::<Vec<_>>();
    prove_program(private_tvm_program(), public, tokens, digests)
}

pub fn verify_private_tvm(public: &[u64], proof: &[u64]) -> Result<(), String> {
    verify_program(private_tvm_program(), private_tvm_input(public)?, proof)
}

fn public_input_v4(words: &[u64]) -> Result<Vec<BFieldElement>, String> {
    if words.len() != PUBLIC_WORDS_V4 {
        return Err("invalid Shield4 public input length".into());
    }
    public_input(&words[..PUBLIC_WORDS])?;
    if words[58] == 0 && words[108..113].iter().all(|&v| v == 0) {
        return Err("zero Shield4 linkability tag".into());
    }
    if words[58] == 1 && words[108..113].iter().any(|&v| v != 0) {
        return Err("deposit must not carry a Shield4 linkability tag".into());
    }
    canonical_words(words, PUBLIC_WORDS_V4)
}

fn spending_paths_v4(public: &[u64], path: &[[u64; 5]]) -> Result<Vec<[u64; 5]>, String> {
    public_input_v4(public)?;
    spending_paths(&public[..PUBLIC_WORDS], path)
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
/// Output: input owner, input commitment, root, nullifier, then each output
/// commitment followed by its one-time key (five words each).
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
    let mut key_words = vec![DOMAIN_NULLIFIER_KEY];
    key_words.extend(&secret[..5]);
    null_words.extend(hash(&key_words).values().map(|v| v.value()));
    null_words.extend(&secret[5..18]);
    let nullifier = hash(&null_words);
    let mut result = Vec::with_capacity(40);
    for digest in [owner, input, root, nullifier] {
        result.extend(digest.values().map(|v| v.value()));
    }
    for i in 0..4 {
        let base = 19 + 18 * i;
        let commitment = note(
            &secret[base..base + 5],
            &secret[base + 5..base + 10],
            &secret[base + 10..base + 18],
        );
        result.extend(commitment.values().map(|v| v.value()));
        let mut key_words = vec![DOMAIN_ONETIME_KEY];
        key_words.extend(&secret[base..base + 5]);
        key_words.extend(&secret[base + 5..base + 10]);
        key_words.extend(commitment.values().map(|v| v.value()));
        let key = hash(&key_words);
        result.extend(key.values().map(|v| v.value()));
    }
    Ok(result)
}

pub fn describe_spend_v4(
    public: &[u64],
    secret: &[u64],
    path: &[[u64; 5]],
) -> Result<Vec<u64>, String> {
    // Description is a witness-only derivation; the relation's domain is
    // carried by the proof operation.  Keep the canonical output derivation
    // identical so Shield3 notes can migrate into the common full-chain tree.
    canonical_words(public, PUBLIC_WORDS_V4)?;
    let mut result = describe_spend(&public[..PUBLIC_WORDS], secret, path)?;
    let mut tag = vec![DOMAIN_SHIELD4_TAG];
    let owner = &result[..5];
    tag.extend(owner);
    tag.extend(&secret[5..10]);
    tag.push(secret[18]);
    tag.extend(&public[28..33]);
    let tag = Tip5::hash_varlen(
        &tag.iter()
            .copied()
            .map(BFieldElement::new)
            .collect::<Vec<_>>(),
    );
    result.splice(20..20, tag.values().map(|v| v.value()));
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
    a.emit("merkle_path: push 32 push 320 write_mem 1 pop 1 call merkle_loop return");
    a.emit("merkle_loop: merkle_step push 320 read_mem 1 pop 1 push -1 add dup 0 push 320 write_mem 1 pop 1 push 0 eq skiz return recurse");
    a.emit(
        "check_range: push 330 write_mem 1 pop 1 push 331 write_mem 1 pop 1 call range_loop return",
    );
    a.emit("range_loop: push 331 read_mem 1 pop 1 dup 0 call check_u32 push 1 add push 331 write_mem 1 pop 1 push 330 read_mem 1 pop 1 push -1 add dup 0 push 330 write_mem 1 pop 1 push 0 eq skiz return recurse");
    a.emit("nullifier_hash: push 400 write_mem 1 pop 1");
    let mut nullifier = vec![Word::Literal(DOMAIN_NULLIFIER)];
    nullifier.extend((0..4).map(Word::Memory));
    nullifier.extend((210..215).map(Word::Memory));
    nullifier.extend((0..13).map(|offset| Word::Pointer(400, offset)));
    a.hash(nullifier);
    a.emit("return");
    a.emit("stamp_hash: push 400 write_mem 1 pop 1");
    let mut stamp = vec![
        Word::Literal(DOMAIN_STAMP),
        Word::Memory(0),
        Word::Memory(1),
    ];
    stamp.extend((0..5).map(|offset| Word::Pointer(400, offset)));
    a.hash(stamp);
    a.emit("return");
    a.note_routine();
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
