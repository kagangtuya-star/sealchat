package service

import "unicode"

// TTSBillableCharacters follows the current Qwen Audio 3.0 TTS character
// accounting rule: Han characters count as two and every other rune as one.
func TTSBillableCharacters(text string) int64 {
	var total int64
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			total += 2
		} else {
			total++
		}
	}
	return total
}
