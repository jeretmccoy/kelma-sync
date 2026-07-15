//! kelma-hash CLI.
//!
//! Modes:
//!   Single (default): stdin is one item, stdout one hex line.
//!   Batch (-batch):    stdin is a JSON array of items, stdout one hex line each.
//!
//! An "item" is either:
//!   - {"parts": [<json value>, ...]}  — general form, matches Go checksum(...any)
//!   - {"fields": [...], "tags": [...]} — note convenience form
//!
//! Batch mode lets a caller checksum an entire collection with one process spawn.

use std::io::{self, Read, Write};

use serde::Deserialize;
use serde_json::Value;

#[derive(Deserialize)]
struct Item {
    parts: Option<Vec<Value>>,
    fields: Option<Vec<String>>,
    tags: Option<Vec<String>>,
}

impl Item {
    fn checksum(&self) -> String {
        if let Some(parts) = &self.parts {
            kelma_hash::checksum_parts(parts)
        } else {
            let fields = self.fields.clone().unwrap_or_default();
            let tags = self.tags.clone().unwrap_or_default();
            kelma_hash::note_checksum(&fields, &tags)
        }
    }
}

fn main() {
    let batch = std::env::args().nth(1).as_deref() == Some("-batch");

    let mut input = String::new();
    if let Err(e) = io::stdin().read_to_string(&mut input) {
        eprintln!("kelma-hash: read: {e}");
        std::process::exit(1);
    }

    let stdout = io::stdout();
    let mut out = io::BufWriter::new(stdout.lock());

    if batch {
        let items: Vec<Item> = match serde_json::from_str(&input) {
            Ok(v) => v,
            Err(e) => {
                eprintln!("kelma-hash: decode batch: {e}");
                std::process::exit(1);
            }
        };
        for it in &items {
            let _ = writeln!(out, "{}", it.checksum());
        }
    } else {
        let it: Item = match serde_json::from_str(&input) {
            Ok(v) => v,
            Err(e) => {
                eprintln!("kelma-hash: decode: {e}");
                std::process::exit(1);
            }
        };
        let _ = writeln!(out, "{}", it.checksum());
    }
    let _ = out.flush();
}
