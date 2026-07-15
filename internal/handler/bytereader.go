package handler

import "bytes"

// byteReader wraps a byte slice as an io.Reader for storage uploads.
func byteReader(b []byte) *bytes.Reader {
	return bytes.NewReader(b)
}
