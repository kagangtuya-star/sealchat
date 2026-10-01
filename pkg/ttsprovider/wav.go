package ttsprovider

import (
	"encoding/binary"
	"errors"
)

// EncodePCM16MonoWAV wraps little-endian mono PCM16 in one bounded RIFF file.
func EncodePCM16MonoWAV(pcm []byte, sampleRate int) ([]byte, error) {
	if len(pcm) == 0 || len(pcm)%2 != 0 || len(pcm)+44 > MaxAudioBytes || (sampleRate != 8000 && sampleRate != 16000 && sampleRate != 24000) {
		return nil, errors.New("无效或过大的 PCM16 mono 音频")
	}
	wav := make([]byte, 44+len(pcm))
	copy(wav, "RIFF")
	binary.LittleEndian.PutUint32(wav[4:], uint32(len(wav)-8))
	copy(wav[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(wav[16:], 16)
	binary.LittleEndian.PutUint16(wav[20:], 1)
	binary.LittleEndian.PutUint16(wav[22:], 1)
	binary.LittleEndian.PutUint32(wav[24:], uint32(sampleRate))
	binary.LittleEndian.PutUint32(wav[28:], uint32(sampleRate*2))
	binary.LittleEndian.PutUint16(wav[32:], 2)
	binary.LittleEndian.PutUint16(wav[34:], 16)
	copy(wav[36:], "data")
	binary.LittleEndian.PutUint32(wav[40:], uint32(len(pcm)))
	copy(wav[44:], pcm)
	return wav, nil
}

// ParsePCM16MonoWAV returns the data chunk after strict complete-file validation,
// including optional metadata chunks and RIFF padding.
func ParsePCM16MonoWAV(wav []byte, sampleRate int) ([]byte, error) {
	if len(wav) > MaxAudioBytes {
		return nil, ErrMedia
	}
	m, err := InspectMedia(wav)
	if err != nil || m.Container != "wav" || m.Codec != "pcm_s16le" || m.ChannelCount != 1 || m.SampleRate != sampleRate {
		return nil, ErrMedia
	}
	return wav[m.DataOffset : m.DataOffset+m.DataSize], nil
}
