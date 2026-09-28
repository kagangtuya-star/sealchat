package ttsprovider

import "encoding/binary"

// InspectWAVStreamHeader recognizes only finite PCM16 RIFF streams. A missing
// header is distinguishable from an unsupported header; SSE boundaries do not
// participate in this parser. Complete archives still require InspectMedia.
func InspectWAVStreamHeader(b []byte) (Media, bool, error) {
	if len(b) < 12 {
		return Media{}, false, nil
	}
	if string(b[:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return Media{}, false, ErrMedia
	}
	end := uint64(binary.LittleEndian.Uint32(b[4:8])) + 8
	if end > MaxAudioBytes || end < 44 {
		return Media{}, false, ErrMedia
	}
	m := Media{Codec: "pcm_s16le", Container: "wav"}
	fmtSeen := false
	for p := 12; ; {
		if p+8 > len(b) {
			return Media{}, false, nil
		}
		n := int(binary.LittleEndian.Uint32(b[p+4:]))
		start := p + 8
		if uint64(start)+uint64(n)+uint64(n%2) > end {
			return Media{}, false, ErrMedia
		}
		switch string(b[p : p+4]) {
		case "fmt ":
			if fmtSeen || n < 16 {
				return Media{}, false, ErrMedia
			}
			if start+n > len(b) {
				return Media{}, false, nil
			}
			if binary.LittleEndian.Uint16(b[start:]) != 1 || binary.LittleEndian.Uint16(b[start+14:]) != 16 {
				return Media{}, false, ErrMedia
			}
			m.ChannelCount = int(binary.LittleEndian.Uint16(b[start+2:]))
			m.SampleRate = int(binary.LittleEndian.Uint32(b[start+4:]))
			if m.ChannelCount < 1 || m.ChannelCount > 2 || m.SampleRate < 8000 || m.SampleRate > 96000 || int(binary.LittleEndian.Uint16(b[start+12:])) != m.ChannelCount*2 || int(binary.LittleEndian.Uint32(b[start+8:])) != m.ChannelCount*2*m.SampleRate {
				return Media{}, false, ErrMedia
			}
			fmtSeen = true
		case "data":
			if !fmtSeen || n == 0 || n%(m.ChannelCount*2) != 0 {
				return Media{}, false, ErrMedia
			}
			m.DataOffset, m.DataSize = start, n
			m.DurationMS = int64(n) * 1000 / int64(m.ChannelCount*2*m.SampleRate)
			return m, true, nil
		}
		p = start + n + n%2
		if p > 64<<10 {
			return Media{}, false, ErrMedia
		}
	}
}
