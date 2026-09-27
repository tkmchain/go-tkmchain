use anyhow::{bail, ensure, Context, Result};
use clap::Parser;
use sha2::{Digest, Sha256};
use std::{env, fs, path::PathBuf};
use zkm_sdk::provers::ProofOpts;
use zkm_sdk::{
    utils, HashableKey, ProverClient, ZKMContext, ZKMProofKind, ZKMProofWithPublicValues, ZKMStdin,
    ZKMVerifyingKey,
};
use zkm_stark::{ZKMCoreOpts, ZKMProverOpts};

/// Parse a positive bounded prover setting. CI can tune recursion without
/// changing the proof protocol or recompiling the guest.
fn prover_setting(name: &str, default: usize, max: usize) -> usize {
    env::var(name)
        .ok()
        .and_then(|value| value.parse::<usize>().ok())
        .filter(|value| *value > 0 && *value <= max)
        .unwrap_or(default)
}

/// Build conservative recursion options for hosted runners. Ziren's default
/// recursion configuration uses two 2M-cycle proving workers, which can exceed
/// the memory envelope of a standard GitHub runner and result in an external
/// runner cancellation. A single bounded worker is deterministic and produces
/// the same proof format; it only trades memory for proving time.
fn keeper_prover_opts() -> ZKMProverOpts {
    let mut core_opts = ZKMCoreOpts::default();
    let mut recursion_opts = ZKMCoreOpts::recursion();
    recursion_opts.shard_size = prover_setting("KEEPER_RECURSION_SHARD_SIZE", 1 << 20, 1 << 21);
    recursion_opts.shard_batch_size = prover_setting("KEEPER_RECURSION_SHARD_BATCH_SIZE", 1, 8);
    recursion_opts.trace_gen_workers = prover_setting("KEEPER_RECURSION_TRACE_GEN_WORKERS", 1, 8);
    recursion_opts.records_and_traces_channel_capacity =
        prover_setting("KEEPER_RECURSION_RECORDS_CHANNEL_CAPACITY", 1, 128);
    recursion_opts.checkpoints_channel_capacity =
        prover_setting("KEEPER_RECURSION_CHECKPOINTS_CHANNEL_CAPACITY", 8, 128);

    // Keep core proving bounded as well when a runner supplies an explicit
    // value. The default remains Ziren's memory-aware setting.
    if let Ok(value) = env::var("KEEPER_CORE_SHARD_SIZE") {
        if let Ok(value) = value.parse::<usize>() {
            if (1 << 15..=1 << 21).contains(&value) {
                core_opts.shard_size = value;
            }
        }
    }

    ZKMProverOpts {
        core_opts,
        recursion_opts,
    }
}

/// Generates and locally verifies a Ziren STARK proof for the complete
/// stateless go-tkmchain EVM execution guest.
///
/// The guest ELF must be built from cmd/keeper with the `ziren` tag for the
/// MIPS32r2 target. The input file is the RLP-encoded keeper Payload.
#[derive(Parser, Debug)]
#[command(author, version, about)]
struct Args {
    /// Ziren MIPS guest ELF produced from cmd/keeper.
    #[arg(long)]
    elf: Option<PathBuf>,
    /// RLP-encoded keeper Payload (block, witness, and chain ID).
    #[arg(long)]
    input: Option<PathBuf>,
    /// Output proof-with-public-values file.
    #[arg(long, default_value = "keeper-proof.bin")]
    proof: PathBuf,
    /// Output serialized program verifying key.
    #[arg(long, default_value = "keeper-vk.bin")]
    vk: PathBuf,
    /// Verify an existing proof instead of generating one.
    #[arg(long)]
    verify: Option<PathBuf>,
    /// Expected chain ID for verifier-side public-statement checking.
    #[arg(long)]
    chain_id: Option<u64>,
    /// Expected block number for verifier-side public-statement checking.
    #[arg(long)]
    block_number: Option<u64>,
    /// Expected block hash (32-byte hex, with an optional 0x prefix).
    #[arg(long)]
    block_hash: Option<String>,
    /// Expected state root (32-byte hex, with an optional 0x prefix).
    #[arg(long)]
    state_root: Option<String>,
    /// Expected receipt root (32-byte hex, with an optional 0x prefix).
    #[arg(long)]
    receipt_root: Option<String>,
}

struct ExpectedStatement {
    chain_id: u64,
    block_number: u64,
    block_hash: [u8; 32],
    state_root: [u8; 32],
    receipt_root: [u8; 32],
}

impl ExpectedStatement {
    fn from_args(args: &Args) -> Result<Option<Self>> {
        let supplied = [
            args.chain_id.is_some(),
            args.block_number.is_some(),
            args.block_hash.is_some(),
            args.state_root.is_some(),
            args.receipt_root.is_some(),
        ];
        if supplied.iter().all(|value| !value) {
            return Ok(None);
        }
        ensure!(
            supplied.iter().all(|value| *value),
            "all five expected statement fields are required together"
        );
        Ok(Some(Self {
            chain_id: args.chain_id.unwrap(),
            block_number: args.block_number.unwrap(),
            block_hash: parse_hash("block-hash", args.block_hash.as_deref().unwrap())?,
            state_root: parse_hash("state-root", args.state_root.as_deref().unwrap())?,
            receipt_root: parse_hash("receipt-root", args.receipt_root.as_deref().unwrap())?,
        }))
    }

    fn encode(&self) -> Vec<u8> {
        let mut encoded = Vec::with_capacity(116);
        encoded.extend_from_slice(&1u32.to_le_bytes());
        encoded.extend_from_slice(&self.chain_id.to_le_bytes());
        encoded.extend_from_slice(&self.block_number.to_le_bytes());
        encoded.extend_from_slice(&self.block_hash);
        encoded.extend_from_slice(&self.state_root);
        encoded.extend_from_slice(&self.receipt_root);
        encoded
    }

    fn public_values_digest(&self) -> Vec<u8> {
        Sha256::digest(self.encode()).to_vec()
    }
}

fn parse_hash(label: &str, value: &str) -> Result<[u8; 32]> {
    let value = value.strip_prefix("0x").unwrap_or(value);
    ensure!(value.len() == 64, "{label} must contain exactly 32 bytes");
    let mut output = [0u8; 32];
    for (index, byte) in output.iter_mut().enumerate() {
        *byte = u8::from_str_radix(&value[index * 2..index * 2 + 2], 16)
            .with_context(|| format!("invalid {label} hex"))?;
    }
    Ok(output)
}

fn verify_statement(
    proof: &ZKMProofWithPublicValues,
    expected: Option<&ExpectedStatement>,
) -> Result<()> {
    let Some(expected) = expected else {
        return Ok(());
    };
    let actual = proof.public_values.as_ref();
    // Ziren's guest runtime hashes every Commit payload and exposes the final
    // SHA-256 digest as the proof public value. Compare that digest rather than
    // treating the raw statement bytes as the public value.
    let wanted = expected.public_values_digest();
    if actual != wanted {
        bail!(
            "public statement mismatch: expected 0x{}, got 0x{}",
            encode_hex(&wanted),
            encode_hex(actual)
        );
    }
    Ok(())
}

fn encode_hex(bytes: &[u8]) -> String {
    const HEX: &[u8; 16] = b"0123456789abcdef";
    let mut output = String::with_capacity(bytes.len() * 2);
    for byte in bytes {
        output.push(HEX[(byte >> 4) as usize] as char);
        output.push(HEX[(byte & 0x0f) as usize] as char);
    }
    output
}

/// Encode a byte slice exactly as the Go zkVM runtime's `SerializeData` does.
///
/// The guest calls `zkvm_runtime.Read[[]byte]`, whose wire format is an
/// unsigned little-endian 64-bit byte length followed by the bytes.  Using
/// `ZKMStdin::write` here happened to produce the same representation with
/// the currently pinned bincode version, but it couples the consensus input
/// protocol to a Rust serializer configuration.  Writing the bytes
/// explicitly keeps the host and guest contracts stable across dependency
/// updates and prevents an RLP payload from being decoded with the wrong
/// offset (the source of the misleading ChainID decode error).
fn encode_guest_bytes(input: &[u8]) -> Result<Vec<u8>> {
    let length = u64::try_from(input.len()).context("keeper payload is too large")?;
    let mut encoded = Vec::with_capacity(8 + input.len());
    encoded.extend_from_slice(&length.to_le_bytes());
    encoded.extend_from_slice(input);
    Ok(encoded)
}

fn validate_payload_prefix(input: &[u8]) -> Result<()> {
    ensure!(!input.is_empty(), "keeper payload is empty");
    ensure!(input[0] >= 0xc0, "keeper payload must be an RLP list");

    // The first field is ChainID.  It must be an RLP scalar, never another
    // list.  This catches a stale guest ELF or a double-wrapped payload before
    // invoking the prover and reports the actual boundary problem.
    let list_payload_offset = if input[0] <= 0xf7 {
        1usize
    } else {
        let length_of_length = usize::from(input[0] - 0xf7);
        ensure!(length_of_length > 0, "invalid keeper RLP list prefix");
        ensure!(
            input.len() > length_of_length,
            "truncated keeper RLP list prefix"
        );
        1 + length_of_length
    };
    ensure!(
        input.len() > list_payload_offset,
        "keeper RLP list has no fields"
    );
    ensure!(
        input[list_payload_offset] < 0xc0,
        "keeper payload ChainID is not the first scalar field"
    );
    Ok(())
}

fn main() -> Result<()> {
    utils::setup_logger();
    let args = Args::parse();
    let expected = ExpectedStatement::from_args(&args)?;
    let client = ProverClient::new();
    if let Some(proof_path) = args.verify {
        let proof = ZKMProofWithPublicValues::load(&proof_path)
            .with_context(|| format!("load proof {}", proof_path.display()))?;
        let vk_bytes = fs::read(&args.vk)
            .with_context(|| format!("read verifying key {}", args.vk.display()))?;
        let vk: ZKMVerifyingKey =
            bincode::deserialize(&vk_bytes).context("decode verifying key")?;
        client
            .verify(&proof, &vk)
            .context("verify keeper STARK proof")?;
        verify_statement(&proof, expected.as_ref())?;
        println!("valid=true");
        println!("program_vk={}", vk.bytes32());
        println!("public_values_len={}", proof.public_values.as_ref().len());
        return Ok(());
    }

    let elf_path = args.elf.context("--elf is required when proving")?;
    let input_path = args.input.context("--input is required when proving")?;
    let elf =
        fs::read(&elf_path).with_context(|| format!("read guest ELF {}", elf_path.display()))?;
    let input =
        fs::read(&input_path).with_context(|| format!("read payload {}", input_path.display()))?;
    validate_payload_prefix(&input)
        .with_context(|| format!("validate payload {}", input_path.display()))?;

    let mut stdin = ZKMStdin::new();
    // getpayload_ziren.go reads []byte.  Write the Go runtime's explicit
    // length-prefixed byte-slice representation, rather than raw RLP.
    let guest_input = encode_guest_bytes(&input)?;
    stdin.write_slice(&guest_input);

    let (_, report) = client
        .execute(&elf, &stdin)
        .run()
        .context("execute keeper guest")?;
    eprintln!(
        "keeper execution cycles: {}",
        report.total_instruction_count()
    );

    let (pk, vk) = client.setup(&elf);
    // Use the low-level prover entry point so the recursion settings can be
    // bounded on hosted runners. The public proof format and verification
    // path are unchanged from `Prove::compressed().run()`.
    let proof = client
        .prover
        .prove_impl(
            &pk,
            stdin,
            ProofOpts {
                zkm_prover_opts: keeper_prover_opts(),
                timeout: None,
            },
            ZKMContext::default(),
            ZKMProofKind::Compressed,
            None,
        )
        .map(|(proof, _cycles)| proof)
        .context("generate keeper STARK proof")?;

    // Verify before writing anything that can be consumed by another tool.
    client
        .verify(&proof, &vk)
        .context("verify keeper STARK proof")?;
    verify_statement(&proof, expected.as_ref())?;
    proof
        .save(&args.proof)
        .with_context(|| format!("write proof {}", args.proof.display()))?;
    fs::write(
        &args.vk,
        bincode::serialize(&vk).context("encode verifying key")?,
    )
    .with_context(|| format!("write verifying key {}", args.vk.display()))?;

    println!("proof_file={}", args.proof.display());
    println!("vk_file={}", args.vk.display());
    println!("program_vk={}", vk.bytes32());
    println!("public_values_len={}", proof.public_values.as_ref().len());
    // Round-trip the serialized artifact and verify it again. This catches
    // proof-file truncation and serialization mismatches before publication.
    let loaded = ZKMProofWithPublicValues::load(&args.proof).context("reload proof")?;
    client
        .verify(&loaded, &vk)
        .context("verify reloaded proof")?;
    verify_statement(&loaded, expected.as_ref())?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn guest_byte_encoding_matches_go_runtime() {
        let input = b"keeper-payload";
        let expected = bincode::serialize(&input.as_slice()).expect("bincode serialization");
        assert_eq!(encode_guest_bytes(input).expect("guest encoding"), expected);
    }

    #[test]
    fn payload_prefix_requires_scalar_chain_id() {
        // A minimal list containing the scalar chain ID 8979.
        assert!(validate_payload_prefix(&[0xc3, 0x82, 0x23, 0x13]).is_ok());
        // A nested list in the ChainID position is the malformed layout that
        // previously reached the guest and produced the confusing RLP error.
        assert!(validate_payload_prefix(&[0xc3, 0xc2, 0x23, 0x13]).is_err());
    }
}
