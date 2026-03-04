package service

// NormalizeMessageContentToPlainText converts stored message content
// (plain text, HTML, TipTap JSON) into readable plain text.
func NormalizeMessageContentToPlainText(input string) string {
	return stripRichText(input)
}
