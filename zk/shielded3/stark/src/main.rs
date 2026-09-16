use std::io::{self, Read, Write};
use tkm_shield3_stark::*;

const MAGIC: &[u8; 8] = b"TKMS3STK";
const MAX_INPUT_BYTES: usize = 8 + PUBLIC_WORDS * 8 + 4 + MAX_PROOF_WORDS * 8;

fn word(input: &mut &[u8]) -> Result<u64, String> {
    if input.len() < 8 {
        return Err("truncated request".into());
    }
    let (head, tail) = input.split_at(8);
    *input = tail;
    Ok(u64::from_le_bytes(head.try_into().unwrap()))
}
fn words(input: &mut &[u8], count: usize) -> Result<Vec<u64>, String> {
    (0..count).map(|_| word(input)).collect()
}
fn run() -> Result<(), String> {
    let mut args = std::env::args().skip(1);
    let mode = args.next().ok_or("missing operation")?;
    if args.next().is_some() || !["prove", "verify", "describe"].contains(&mode.as_str()) {
        return Err("invalid operation".into());
    }
    let mut data = Vec::new();
    io::stdin()
        .take((MAX_INPUT_BYTES + 1) as u64)
        .read_to_end(&mut data)
        .map_err(|_| "input read failed")?;
    if data.len() > MAX_INPUT_BYTES || data.get(..8) != Some(MAGIC) {
        return Err("invalid request".into());
    }
    let mut input = &data[8..];
    let public = words(&mut input, PUBLIC_WORDS)?;
    if mode == "verify" {
        if input.len() < 4 {
            return Err("truncated proof length".into());
        }
        let count = u32::from_le_bytes(input[..4].try_into().unwrap()) as usize;
        input = &input[4..];
        if count == 0 || count > MAX_PROOF_WORDS || input.len() != count * 8 {
            return Err("invalid proof length".into());
        }
        let proof = words(&mut input, count)?;
        verify_spend(&public, &proof)?;
        io::stdout()
            .write_all(b"OK\n")
            .map_err(|_| "output write failed")?;
    } else {
        let secret = words(&mut input, SECRET_WORDS)?;
        let mut path = Vec::with_capacity(MERKLE_DEPTH);
        for _ in 0..MERKLE_DEPTH {
            path.push(words(&mut input, 5)?.try_into().unwrap());
        }
        if !input.is_empty() {
            return Err("trailing request data".into());
        }
        if mode == "describe" {
            let result = describe_spend(&public, &secret, &path)?;
            let mut out = io::stdout().lock();
            for v in result {
                out.write_all(&v.to_le_bytes())
                    .map_err(|_| "output write failed")?;
            }
            return Ok(());
        }
        let proof = prove_spend(&public, &secret, &path)?;
        let mut out = io::stdout().lock();
        out.write_all(&(proof.len() as u32).to_le_bytes())
            .map_err(|_| "output write failed")?;
        for v in proof {
            out.write_all(&v.to_le_bytes())
                .map_err(|_| "output write failed")?;
        }
    }
    Ok(())
}
fn main() {
    // Hostile input must produce a nonzero exit, never accidental success.
    std::panic::set_hook(Box::new(|_| {}));
    match std::panic::catch_unwind(run) {
        Ok(Ok(())) => {}
        Ok(Err(e)) => {
            eprintln!("{e}");
            std::process::exit(2);
        }
        Err(_) => {
            eprintln!("malformed STARK input");
            std::process::exit(2);
        }
    }
}
