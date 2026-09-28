package ttsprovider

import (
	"encoding/binary"
	"errors"
)

type Media struct {
	Codec        string `json:"codec"`
	Container    string `json:"container"`
	SampleRate   int    `json:"sampleRate"`
	ChannelCount int    `json:"channelCount"`
	DurationMS   int64  `json:"durationMs"`
	DataOffset   int    `json:"-"`
	DataSize     int    `json:"-"`
}

var ErrMedia = errors.New("音频容器不受支持或文件不完整；不会自动重试收费合成")

// InspectMedia validates a complete archive, independently of SSE completion.
// PCM WAV is the only realtime container. Other validated media use file mode.
func InspectMedia(b []byte) (Media, error) {
	if len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WAVE" {
		if uint64(binary.LittleEndian.Uint32(b[4:8]))+8 != uint64(len(b)) {
			return Media{}, ErrMedia
		}
		m := Media{Codec: "pcm_s16le", Container: "wav"}
		alignment := 0
		p := 12
		for p+8 <= len(b) {
			n := int(binary.LittleEndian.Uint32(b[p+4 : p+8]))
			start := p + 8
			if n < 0 || n > len(b)-start {
				return Media{}, ErrMedia
			}
			switch string(b[p : p+4]) {
			case "fmt ":
				if alignment != 0 || n < 16 || binary.LittleEndian.Uint16(b[start:]) != 1 || binary.LittleEndian.Uint16(b[start+14:]) != 16 {
					return Media{}, ErrMedia
				}
				m.ChannelCount = int(binary.LittleEndian.Uint16(b[start+2:]))
				m.SampleRate = int(binary.LittleEndian.Uint32(b[start+4:]))
				alignment = int(binary.LittleEndian.Uint16(b[start+12:]))
				if uint64(binary.LittleEndian.Uint32(b[start+8:])) != uint64(alignment)*uint64(m.SampleRate) {
					return Media{}, ErrMedia
				}
			case "data":
				if m.DataOffset != 0 {
					return Media{}, ErrMedia
				}
				m.DataOffset = start
				m.DataSize = n
			}
			p = start + n + (n % 2)
		}
		if p != len(b) || m.SampleRate < 8000 || m.SampleRate > 96000 || m.ChannelCount < 1 || m.ChannelCount > 2 || alignment != m.ChannelCount*2 || m.DataSize == 0 || m.DataSize%alignment != 0 {
			return Media{}, ErrMedia
		}
		m.DurationMS = int64(m.DataSize) * 1000 / int64(alignment*m.SampleRate)
		return m, nil
	}
	// MPEG layer III frame validation, including the final frame length.
	p := 0
	if len(b) >= 10 && string(b[:3]) == "ID3" {
		for _, v := range b[6:10] {
			if v&0x80 != 0 {
				return Media{}, ErrMedia
			}
		}
		p = 10 + (int(b[6]) << 21) + (int(b[7]) << 14) + (int(b[8]) << 7) + int(b[9])
	}
	m := Media{Codec: "mp3", Container: "mp3"}
	samples := int64(0)
	for p+4 <= len(b) {
		if len(b)-p == 128 && string(b[p:p+3]) == "TAG" {
			p = len(b)
			break
		}
		h := binary.BigEndian.Uint32(b[p : p+4])
		version := (h >> 19) & 3
		if h>>21 != 0x7ff || version == 1 || (h>>17)&3 != 1 {
			return Media{}, ErrMedia
		}
		idx := (h >> 12) & 15
		sridx := (h >> 10) & 3
		if idx == 0 || idx == 15 || sridx == 3 {
			return Media{}, ErrMedia
		}
		rate := []int{44100, 48000, 32000}[sridx]
		bitrates := []int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320}
		coeff, frameSamples := 144000, 1152
		if version != 3 {
			rate /= 2
			if version == 0 {
				rate /= 2
			}
			bitrates = []int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160}
			coeff = 72000
			frameSamples = 576
		}
		channels := 2
		if (h>>6)&3 == 3 {
			channels = 1
		}
		if m.SampleRate != 0 && (m.SampleRate != rate || m.ChannelCount != channels) {
			return Media{}, ErrMedia
		}
		m.SampleRate = rate
		m.ChannelCount = channels
		n := coeff*bitrates[idx]/rate + int((h>>9)&1)
		if n > len(b)-p {
			return Media{}, ErrMedia
		}
		p += n
		samples += int64(frameSamples)
	}
	if p != len(b) || samples == 0 {
		return Media{}, ErrMedia
	}
	m.DurationMS = samples * 1000 / int64(m.SampleRate)
	return m, nil
}
