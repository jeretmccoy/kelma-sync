package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// checksum computes a stable content digest over the given parts.
//
// This is byte-identical to the canonical Rust `kelma-hash` implementation
// (used by KelmaMobile) and to the Python plugin client — verified by
// TestChecksumParityWithRust. serde_json (Rust), Go's json.Encoder with
// SetEscapeHTML(false), and Python's json.dumps(ensure_ascii=False,
// separators=(",",":"),sort_keys=True) all produce the same compact, raw-UTF-8,
// sorted-key JSON, followed by a newline per part.
//
// The server uses this native Go implementation directly (no subprocess) for
// efficiency; the Rust crate remains the canonical spec that this is tested
// against.
func checksum(parts ...any) string {
	h := sha256.New()
	enc := json.NewEncoder(h)
	enc.SetEscapeHTML(false)
	for _, p := range parts {
		_ = enc.Encode(p)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// checksumNative is an alias kept so the parity test can compare the production
// implementation explicitly against the Rust binary.
func checksumNative(parts ...any) string {
	return checksum(parts...)
}

// checksumBatch computes checksums for many items (each a slice of parts),
// preserving order. Pure Go; no subprocess.
func checksumBatch(items [][]any) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, len(items))
	for i, parts := range items {
		out[i] = checksum(parts...)
	}
	return out
}

// normalizeDeckConfig removes volatile local-bookkeeping fields from a deck
// config before checksumming. These fields differ across clients and must not
// cause sync conflicts.
func normalizeDeckConfig(cfg map[string]any) map[string]any {
	if cfg == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(cfg))
	for k, v := range cfg {
		switch k {
		case "id", "mod", "usn", "name", "newToday", "revToday", "lrnToday", "timeToday":
			continue
		}
		out[k] = v
	}
	return out
}

// normalizeNotetypeDefinition removes volatile local-bookkeeping fields from a
// notetype definition before checksumming. This includes deep-normalization of
// auto-generated ids inside flds/tmpls array entries, which differ per-client.
func normalizeNotetypeDefinition(def map[string]any) map[string]any {
	if def == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(def))
	for k, v := range def {
		switch k {
		case "id", "mod", "usn":
			continue
		}
		if k == "flds" || k == "tmpls" {
			if arr, ok := v.([]any); ok {
				newArr := make([]any, len(arr))
				for i, item := range arr {
					if m, ok := item.(map[string]any); ok {
						cleaned := make(map[string]any, len(m))
						for mk, mv := range m {
							if mk == "id" {
								continue
							}
							cleaned[mk] = mv
						}
						newArr[i] = cleaned
					} else {
						newArr[i] = item
					}
				}
				out[k] = newArr
				continue
			}
		}
		out[k] = v
	}
	return out
}

// deckChecksum computes a normalized checksum for a deck config.
func deckChecksum(cfg map[string]any) string {
	return checksum(normalizeDeckConfig(cfg))
}

// reviewChecksum identifies immutable review content without including the
// collection-local source card id or current deck metadata. A review relayed by
// another client therefore remains byte-identical after its card id is mapped.
func reviewChecksum(
	noteGUID string,
	cardOrd int,
	ease int,
	interval int,
	lastInterval int,
	factor int,
	takenMillis int,
	reviewKind int,
) string {
	return checksum(
		noteGUID,
		cardOrd,
		ease,
		interval,
		lastInterval,
		factor,
		takenMillis,
		reviewKind,
	)
}

// notetypeChecksum computes a normalized checksum for a notetype.
func notetypeChecksum(name string, def map[string]any) string {
	return checksum(name, normalizeNotetypeDefinition(def))
}
