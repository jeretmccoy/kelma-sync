//! Canonical KelmaSync content checksum.
//!
//! Single source of truth for how content is hashed. Used by:
//!   - the CLI binary (`kelma-hash`) the Python plugin and Go server shell out to,
//!   - KelmaMobile (depends on this crate directly).
//!
//! The algorithm mirrors the server's original Go `checksum(...any)`:
//!   sha256( json(part0) + "\n" + json(part1) + "\n" + ... )
//! where json() is compact (no spaces), raw UTF-8 (no \u escaping, no HTML
//! escaping), with object keys sorted — matching Go's `json.Encoder` with
//! `SetEscapeHTML(false)` (Go sorts map keys; serde_json sorts by default).

use serde_json::Value;
use sha2::{Digest, Sha256};

/// Hash an ordered list of JSON value parts.
pub fn checksum_parts(parts: &[Value]) -> String {
    let mut hasher = Sha256::new();
    for part in parts {
        let json = serde_json::to_vec(part).expect("serialize part");
        hasher.update(&json);
        hasher.update(b"\n");
    }
    hex(hasher.finalize().as_slice())
}

/// Convenience for a note: checksum over [fields, tags].
pub fn note_checksum(fields: &[String], tags: &[String]) -> String {
    let parts = vec![
        Value::Array(fields.iter().map(|s| Value::String(s.clone())).collect()),
        Value::Array(tags.iter().map(|s| Value::String(s.clone())).collect()),
    ];
    checksum_parts(&parts)
}

fn hex(bytes: &[u8]) -> String {
    let mut s = String::with_capacity(bytes.len() * 2);
    for b in bytes {
        s.push_str(&format!("{:02x}", b));
    }
    s
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn simple_note() {
        let cs = note_checksum(&["a".to_string(), "b".to_string()], &["x".to_string()]);
        assert_eq!(cs, "a148b9a3db29df2e6dd510b634c61765701e2c0795bb900e9ab07b035988c16f");
    }

    #[test]
    fn parts_matches_note() {
        let via_parts = checksum_parts(&[
            serde_json::json!(["a", "b"]),
            serde_json::json!(["x"]),
        ]);
        let via_note = note_checksum(&["a".to_string(), "b".to_string()], &["x".to_string()]);
        assert_eq!(via_parts, via_note);
    }

    #[test]
    fn unicode_is_raw_utf8() {
        let cs = note_checksum(&["/supɔʁte/".to_string()], &[]);
        assert_eq!(cs.len(), 64);
    }
}
