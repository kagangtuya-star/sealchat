package ttsprovider

import "unicode"

// BillingCharactersHanDouble is shared by Qwen Audio 3.0 and Tencent MPS MiniMax.
func BillingCharactersHanDouble(text string) int64 {
	var total int64
	for _, r := range text {
		total++
		if unicode.Is(unicode.Han, r) {
			total++
		}
	}
	return total
}
