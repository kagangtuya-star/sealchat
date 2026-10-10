package service

import "sealchat/pkg/ttsprovider"

// TTSBillableCharacters implements the Han-double character accounting
// currently shared by Qwen Audio 3.0 and Tencent MPS MiniMax.
func TTSBillableCharacters(text string) int64 {
	return ttsprovider.BillingCharactersHanDouble(text)
}
