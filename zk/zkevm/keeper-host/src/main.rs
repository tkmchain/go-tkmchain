use anyhow::{bail, ensure, Context, Result};
use sha2::{Digest, Sha256};
use clap::Parser;
use std::{fs, path::PathBuf};
use zkm_sdk::{utils, HashableKey, ProverClient, ZKMProofWithPublicValues, ZKMStdin, ZKMVerifyingKey};

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
    let Some(expected) = expected else { return Ok(()) };
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
        let vk: ZKMVerifyingKey = bincode::deserialize(&vk_bytes).context("decode verifying key")?;
        client.verify(&proof, &vk).context("verify keeper STARK proof")?;
        verify_statement(&proof, expected.as_ref())?;
        println!("valid=true");
        println!("program_vk={}", vk.bytes32());
        println!("public_values_len={}", proof.public_values.as_ref().len());
        return Ok(());
    }

    let elf_path = args.elf.context("--elf is required when proving")?;
    let input_path = args.input.context("--input is required when proving")?;
    let elf = fs::read(&elf_path).with_context(|| format!("read guest ELF {}", elf_path.display()))?;
    let input = fs::read(&input_path).with_context(|| format!("read payload {}", input_path.display()))?;

    let mut stdin = ZKMStdin::new();
    // getpayload_ziren.go reads []byte, so the host must serialize a byte
    // slice rather than writing the raw RLP directly.
    stdin.write(&input);

    let (_, report) = client
        .execute(&elf, &stdin)
        .run()
        .context("execute keeper guest")?;
    eprintln!("keeper execution cycles: {}", report.total_instruction_count());

    let (pk, vk) = client.setup(&elf);
    let proof = client
        .prove(&pk, stdin)
        .compressed()
        .run()
        .context("generate keeper STARK proof")?;

    // Verify before writing anything that can be consumed by another tool.
    client.verify(&proof, &vk).context("verify keeper STARK proof")?;
    verify_statement(&proof, expected.as_ref())?;
    proof
        .save(&args.proof)
        .with_context(|| format!("write proof {}", args.proof.display()))?;
    fs::write(&args.vk, bincode::serialize(&vk).context("encode verifying key")?)
        .with_context(|| format!("write verifying key {}", args.vk.display()))?;

    println!("proof_file={}", args.proof.display());
    println!("vk_file={}", args.vk.display());
    println!("program_vk={}", vk.bytes32());
    println!("public_values_len={}", proof.public_values.as_ref().len());
    // Round-trip the serialized artifact and verify it again. This catches
    // proof-file truncation and serialization mismatches before publication.
    let loaded = ZKMProofWithPublicValues::load(&args.proof).context("reload proof")?;
    client.verify(&loaded, &vk).context("verify reloaded proof")?;
    verify_statement(&loaded, expected.as_ref())?;
    Ok(())
}
