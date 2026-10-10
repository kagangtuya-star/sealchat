package ttsprovider

import (
	"encoding/binary"
	"math"
)

// FinalizePCM16WAV seals a complete PCM16 RIFF/WAVE file whose header carries
// estimated or streaming (0xffffffff) lengths. Chunks are walked by the actual
// byte length; only RIFF.size and a trailing data.size are rewritten, on a
// copy. The result must still pass strict InspectMedia, which is never relaxed.
// Non-WAV input and any structure that cannot be determined return ErrMedia.
func FinalizePCM16WAV(b []byte) ([]byte, error) {
	if _, err := InspectMedia(b); err == nil {
		return b, nil
	}
	if len(b) < 44 || uint64(len(b)-8) > math.MaxUint32 || string(b[:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return nil, ErrMedia
	}
	out := append([]byte(nil), b...)
	alignment := 0
	p := 12
	for p+8 <= len(out) {
		n := uint64(binary.LittleEndian.Uint32(out[p+4 : p+8]))
		start := p + 8
		switch string(out[p : p+4]) {
		case "fmt ":
			if alignment != 0 || n < 16 || n > uint64(len(out)-start) {
				return nil, ErrMedia
			}
			channels := int(binary.LittleEndian.Uint16(out[start+2:]))
			rate := uint64(binary.LittleEndian.Uint32(out[start+4:]))
			blockAlign := int(binary.LittleEndian.Uint16(out[start+12:]))
			if binary.LittleEndian.Uint16(out[start:]) != 1 || binary.LittleEndian.Uint16(out[start+14:]) != 16 ||
				channels < 1 || channels > 2 || rate < 8000 || rate > 96000 || blockAlign != channels*2 ||
				uint64(binary.LittleEndian.Uint32(out[start+8:])) != uint64(blockAlign)*rate {
				return nil, ErrMedia
			}
			alignment = blockAlign
		case "data":
			// Streaming WAV puts fmt before data; the payload then runs to EOF
			// unless a valid declared size is followed by complete, named
			// chunks only. Zero or unaligned sizes are always estimates.
			actual := len(out) - start
			if alignment == 0 || actual <= 0 {
				return nil, ErrMedia
			}
			if n > 0 && n < uint64(actual) && n%uint64(alignment) == 0 && wavChunksReachEnd(out, start+int(n)) {
				break
			}
			if actual%alignment != 0 {
				return nil, ErrMedia
			}
			binary.LittleEndian.PutUint32(out[p+4:p+8], uint32(actual))
			n = uint64(actual)
		default:
			if n > uint64(len(out)-start) {
				return nil, ErrMedia
			}
		}
		p = start + int(n) + int(n%2)
	}
	binary.LittleEndian.PutUint32(out[4:8], uint32(len(out)-8))
	if _, err := InspectMedia(out); err != nil {
		return nil, ErrMedia
	}
	return out, nil
}

// wavChunksReachEnd reports whether b[p:] is a sequence of complete chunks with
// printable FourCC IDs ending exactly at EOF, so trailing PCM (including
// silence) is not mistaken for metadata.
func wavChunksReachEnd(b []byte, p int) bool {
	if p >= len(b) {
		return false
	}
	for p+8 <= len(b) {
		for _, c := range b[p : p+4] {
			if c < 0x20 || c > 0x7e {
				return false
			}
		}
		n := uint64(binary.LittleEndian.Uint32(b[p+4 : p+8]))
		start := p + 8
		if n > uint64(len(b)-start) {
			return false
		}
		p = start + int(n) + int(n%2)
	}
	return p == len(b)
}
