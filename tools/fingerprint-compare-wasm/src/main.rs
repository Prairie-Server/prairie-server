//! Prairie fingerprint-compare WASI helper.
//!
//! Reads a JSON compare request from `--input`, writes a JSON response to stdout.
//! Built for wasm32-wasip1 and run in-process by `internal/intromarkers` via wazero.

use std::fs;
use std::path::PathBuf;
use std::process::ExitCode;

use clap::Parser;
use fingerprint_compare_wasm::{compare_fingerprints, CompareRequest};

#[derive(Debug, Parser)]
#[command(
    name = "fingerprint-compare-wasm",
    about = "Prairie WASI chromaprint fingerprint compare helper"
)]
struct Args {
    #[arg(long)]
    input: PathBuf,
}

fn main() -> ExitCode {
    match run() {
        Ok(()) => ExitCode::SUCCESS,
        Err(err) => {
            eprintln!("fingerprint-compare-wasm: {err}");
            ExitCode::FAILURE
        }
    }
}

fn run() -> Result<(), String> {
    let args = Args::parse();
    let raw = fs::read_to_string(&args.input).map_err(|e| format!("read input: {e}"))?;
    let req: CompareRequest =
        serde_json::from_str(&raw).map_err(|e| format!("parse request: {e}"))?;
    let resp = compare_fingerprints(&req);
    let json = serde_json::to_string(&resp).map_err(|e| format!("encode response: {e}"))?;
    println!("{json}");
    Ok(())
}
