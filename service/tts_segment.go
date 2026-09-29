package service

import "unicode"

const (
	ttsSegmentTargetRunes = 200
	ttsSegmentMinRunes    = 160
	ttsSegmentMaxRunes    = 250
)

// TTSSplitText splits only provider-sized long text. It preserves every rune in
// order; punctuation and whitespace stay on the preceding segment.
func TTSSplitText(text string) []string {
	runes := []rune(text)
	if len(runes) <= ttsSegmentMaxRunes {
		return []string{text}
	}

	segments := make([]string, 0, (len(runes)+ttsSegmentTargetRunes-1)/ttsSegmentTargetRunes)
	for len(runes) > ttsSegmentMaxRunes {
		cut := ttsSegmentBoundary(runes, true)
		if cut == 0 {
			cut = ttsSegmentBoundary(runes, false)
		}
		if cut == 0 {
			cut = ttsSegmentTargetRunes
		}
		segments = append(segments, string(runes[:cut]))
		runes = runes[cut:]
	}
	segments = append(segments, string(runes))
	return segments
}

func ttsSegmentBoundary(runes []rune, strong bool) int {
	best, bestDistance := 0, ttsSegmentMaxRunes
	limit := min(len(runes), ttsSegmentMaxRunes)
	for i := ttsSegmentMinRunes - 1; i < limit; i++ {
		boundary := false
		if strong {
			boundary = ttsStrongSentenceEnd(runes, i)
			// Keep consecutive terminal punctuation in the same segment.
			if boundary && i+1 < len(runes) && ttsStrongSentenceEnd(runes, i+1) {
				continue
			}
		} else {
			boundary = ttsWeakBoundary(runes[i])
		}
		if !boundary {
			continue
		}
		cut := i + 1
		distance := cut - ttsSegmentTargetRunes
		if distance < 0 {
			distance = -distance
		}
		if best == 0 || distance < bestDistance {
			best, bestDistance = cut, distance
		}
	}
	return best
}

func ttsStrongSentenceEnd(runes []rune, i int) bool {
	switch runes[i] {
	case '。', '！', '!', '？', '?', '；', ';', '\n':
		return true
	case '.':
		var previous, next rune
		if i > 0 {
			previous = runes[i-1]
		}
		if i+1 < len(runes) {
			next = runes[i+1]
		}
		// Decimal points, domains and other uninterrupted ASCII tokens are not
		// sentence endings. A dot before whitespace/non-ASCII text still is.
		return !(ttsASCIIAlphaNumeric(previous) && ttsASCIIAlphaNumeric(next))
	}
	return false
}

func ttsASCIIAlphaNumeric(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
}

func ttsWeakBoundary(r rune) bool {
	switch r {
	case '，', ',', '、', '：', ':':
		return true
	}
	return unicode.IsSpace(r)
}
