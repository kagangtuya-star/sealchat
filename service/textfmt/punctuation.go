package textfmt

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Single scanner for machine syntax and chat visuals. Keep aligned with frontend.
var protectedPattern = regexp.MustCompile("(?is:```.*?(?:```|\\z))|(?-s:`[^`\\r\\n]*`)|(?i:!?\\[[^\\]\\r\\n]*\\]\\([^\\s\\r\\n]+\\))|(?i:https?://[A-Z0-9:/?#\\[\\]@!$&'()*+,;=%._~\\-]+)|(?i:\\bwww\\.[A-Z0-9:/?#\\[\\]@!$&'()*+,;=%._~\\-]+)|(?i:\\b[A-Z0-9._%+\\-]+@[A-Z0-9.\\-]+\\.[A-Z]{2,}\\b)|(?i:\\b[A-Z0-9._%+\\-]+@[A-Z0-9.\\-]+:/[^\\s，。！？；]+)|(?i:\\b(?:[0-9]{1,3}\\.){3}[0-9]{1,3}(?::[0-9]+)?\\b)|(?i:\\b(?:[0-9A-F]{1,4}:){2,7}[0-9A-F]{1,4}\\b|\\b(?:[0-9A-F]{1,4}:){1,6}:[0-9A-F]{1,4}\\b)|(?i:\\b[A-Z]:\\\\[^\\s，。！？；]+)|(?i:(?:^|[\\s（(])(?:\\.{1,2}/|/)[A-Z0-9._~%+\\-]+(?:/[A-Z0-9._~%+\\-]+)+)|(?i:\\b[A-Z0-9._~%+\\-]+(?:/[A-Z0-9._~%+\\-]+)+\\b)|(?i:\\bv?[0-9]+(?:\\.[0-9]+)+(?:[-+][A-Z0-9.-]+)?\\b)|\\b[0-9]{4}[-/][0-9]{1,2}[-/][0-9]{1,2}\\b|\\b[0-9]{1,2}:[0-9]{2}(?::[0-9]{2})?\\b|\\b[0-9]+:[0-9]+\\b|(?i:\\b[0-9]+x[0-9]+\\b)|\\b[0-9]{1,3}(?:,[0-9]{3})+(?:\\.[0-9]+)?\\b|\\b[0-9]+\\.[0-9]+\\b|(?i:\\b[A-Z_][A-Z0-9_]*(?:::[A-Z_][A-Z0-9_]*)+\\b)|(?i:\\b[A-Z_][A-Z0-9_]*(?:\\.[A-Z_][A-Z0-9_]*)+\\b)|(?i:\\b[A-Z]+(?:'[A-Z]+)+\\b|\\b[A-Z]+'\\b|(?:^|\\s)'[0-9]{2}s\\b|\\brock\\s+'n'\\s+roll\\b)|\\.{3,}|-{2,}|_{3,}|(?:(?:[:;=8xX][-^']?[)(/DPp])|(?:[:;][-^']?\\()|XD)|\\([^()\\r\\n]*(?:・|；|∀|ω|□|°|´|｀|╯|┻|━)[^()\\r\\n]*\\)(?:[^\\r\\n]*┻━┻)?|[～〜~]{2,}|[●○]{2,}|\\\\[\"'?!:.,;]|(?i:<\\/?[A-Z][^>\\r\\n]*>)|(?i:\\[/?[A-Z][^\\]\\r\\n]*\\])|(?i:\\[\\[(?:IMG|IMAGE|ATTACHMENT|EMOJI|图片):[^\\]\\r\\n]+\\]\\])")

var halfToFull = map[rune]rune{',': '，', '.': '。', ';': '；', ':': '：', '?': '？', '!': '！'}
var safePunctuationNormalization = map[rune]string{'⁇': "？？", '‼': "！！", '⁈': "？！", '⁉': "！？"}
var asciiPause = runeSet(",.;:?!")
var fullwidthPause = runeSet("，。、；：？！")
var leftFullwidthMark = runeSet("“‘「『（［｛〔【〖《〈")
var rightFullwidthMark = runeSet("”’」』）］｝〕】〗》〉")
var nonWesternPunctuation = runeSet("，。、；：？！⁈⁇‼⁉“”‘’（）〔〕［］｛｝《》〈〉「」『』【】〖〗—⸺…")

func runeSet(value string) map[rune]struct{} {
	result := make(map[rune]struct{})
	for _, char := range value {
		result[char] = struct{}{}
	}
	return result
}
func contains(set map[rune]struct{}, char rune) bool { _, ok := set[char]; return ok }
func isWestern(char rune) bool {
	return char == '_' || unicode.Is(unicode.Latin, char) || unicode.Is(unicode.Greek, char) || unicode.IsNumber(char)
}
func isFullwidthContext(char rune) bool {
	return unicode.Is(unicode.Han, char) || contains(fullwidthPause, char) || contains(leftFullwidthMark, char) || contains(rightFullwidthMark, char)
}

func isMachineDocument(text string) bool {
	trimmed := strings.TrimSpace(text)
	return trimmed != "" && (trimmed[0] == '{' || trimmed[0] == '[') && json.Valid([]byte(trimmed))
}

type protectedSpan struct{ start, end int }

func trimURLBoundary(value string) int {
	lower := strings.ToLower(value)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") && !strings.HasPrefix(lower, "www.") {
		return len(value)
	}
	end := len(value)
	openParen, closeParen := strings.Count(value, "("), strings.Count(value, ")")
	openBracket, closeBracket := strings.Count(value, "["), strings.Count(value, "]")
	for end > 0 {
		prefix := value[:end]
		char, size := utf8.DecodeLastRuneInString(prefix)
		switch char {
		case '.', ',', ';', ':', '!', '?':
			end -= size
		case ')':
			if closeParen > openParen {
				end -= size
				closeParen--
				continue
			}
			return end
		case ']':
			if closeBracket > openBracket {
				end -= size
				closeBracket--
				continue
			}
			return end
		default:
			return end
		}
	}
	return end
}

func scanProtectedSpans(text string) []protectedSpan {
	matches := protectedPattern.FindAllStringIndex(text, -1)
	spans := make([]protectedSpan, 0, len(matches))
	for _, match := range matches {
		start, end := match[0], match[1]
		end = start + trimURLBoundary(text[start:end])
		if end <= start {
			continue
		}
		if len(spans) > 0 && start <= spans[len(spans)-1].end {
			if end > spans[len(spans)-1].end {
				spans[len(spans)-1].end = end
			}
			continue
		}
		spans = append(spans, protectedSpan{start, end})
	}
	return spans
}

func hasNonWesternContent(text string) bool {
	for _, char := range text {
		if unicode.Is(unicode.Han, char) || contains(nonWesternPunctuation, char) {
			return true
		}
	}
	return false
}

type quoteFamily struct {
	left, right rune
	kind        string
	neutral     rune
}

var quoteFamilies = []quoteFamily{
	{'“', '”', "double-curly", '"'}, {'‘', '’', "single-curly", '\''}, {'「', '」', "corner", '"'},
	{'『', '』', "double-corner", '"'}, {'《', '》', "book", '"'}, {'〈', '〉', "single-book", '\''},
	{'【', '】', "square", '"'}, {'〖', '〗', "double-square", '"'}, {'〔', '〕', "tortoise-shell", '"'},
}

func findQuoteFamily(char rune, left bool) *quoteFamily {
	for index := range quoteFamilies {
		family := &quoteFamilies[index]
		if left && family.left == char || !left && family.right == char {
			return family
		}
	}
	return nil
}
func defaultQuoteFamily(neutral rune) *quoteFamily {
	if neutral == '\'' {
		return &quoteFamilies[1]
	}
	return &quoteFamilies[0]
}

type quotationGroup struct {
	start   int
	neutral rune
	family  *quoteFamily
}

func isApostrophe(chars []rune, index int) bool {
	var previous, next rune
	if index > 0 {
		previous = chars[index-1]
	}
	if index+1 < len(chars) {
		next = chars[index+1]
	}
	if isWestern(previous) && isWestern(next) {
		return true
	}
	if isWestern(previous) && (next == 0 || unicode.IsSpace(next) || unicode.IsPunct(next)) {
		return true
	}
	return unicode.IsDigit(next) && (previous == 0 || unicode.IsSpace(previous) || unicode.IsPunct(previous))
}
func neutralDirection(chars []rune, index int) int {
	var previous, next rune
	if index > 0 {
		previous = chars[index-1]
	}
	if index+1 < len(chars) {
		next = chars[index+1]
	}
	if previous == 0 || contains(leftFullwidthMark, previous) || strings.ContainsRune("([{：:，,；;！？!?\n\r", previous) {
		return 1
	}
	if next == 0 || contains(rightFullwidthMark, next) || strings.ContainsRune(")]，,。.;；:：！？!?\n\r", next) {
		return -1
	}
	return 0
}

func normalizeQuotationGroups(text string) string {
	chars := []rune(text)
	stack := make([]quotationGroup, 0, 4)
	for index, quote := range chars {
		leftFamily, rightFamily := findQuoteFamily(quote, true), findQuoteFamily(quote, false)
		neutral := quote == '"' || quote == '\''
		if !neutral && leftFamily == nil && rightFamily == nil {
			continue
		}
		if quote == '\'' && isApostrophe(chars, index) {
			continue
		}
		if neutral {
			direction := neutralDirection(chars, index)
			if direction == 0 && len(stack) > 0 && stack[len(stack)-1].neutral == quote {
				direction = -1
			}
			if direction < 0 {
				if len(stack) == 0 || stack[len(stack)-1].neutral != quote {
					continue
				}
				active := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				family := active.family
				if family == nil {
					family = defaultQuoteFamily(quote)
					chars[active.start] = family.left
				}
				chars[index] = family.right
				continue
			}
			stack = append(stack, quotationGroup{start: index, neutral: quote})
			continue
		}
		if leftFamily != nil {
			stack = append(stack, quotationGroup{start: index, neutral: leftFamily.neutral, family: leftFamily})
			continue
		}
		if len(stack) == 0 {
			continue
		}
		active := stack[len(stack)-1]
		if active.family == rightFamily {
			stack = stack[:len(stack)-1]
		} else if active.family == nil && active.neutral == rightFamily.neutral {
			chars[active.start] = rightFamily.left
			stack = stack[:len(stack)-1]
		}
	}
	return string(chars)
}

func lineContext(chars []rune) []bool {
	context := make([]bool, len(chars))
	start, found := 0, false
	for index, char := range chars {
		if isFullwidthContext(char) {
			found = true
		}
		if char == '\n' || char == '\r' {
			for i := start; i < index; i++ {
				context[i] = found
			}
			start, found = index+1, false
		}
	}
	for i := start; i < len(chars); i++ {
		context[i] = found
	}
	return context
}
func shouldFullwidth(chars []rune, context []bool, index int) bool {
	if !contains(asciiPause, chars[index]) || !context[index] {
		return false
	}
	var left, right rune
	if index > 0 {
		left = chars[index-1]
	}
	if index+1 < len(chars) {
		right = chars[index+1]
	}
	return !(isWestern(left) && isWestern(right))
}
func normalizeSpaces(text string) string {
	chars := []rune(text)
	out := make([]rune, 0, len(chars))
	for index := 0; index < len(chars); index++ {
		if chars[index] != ' ' {
			out = append(out, chars[index])
			continue
		}
		end := index
		for end+1 < len(chars) && chars[end+1] == ' ' {
			end++
		}
		var previous, next rune
		if len(out) > 0 {
			previous = out[len(out)-1]
		}
		if end+1 < len(chars) {
			next = chars[end+1]
		}
		if contains(fullwidthPause, next) || contains(rightFullwidthMark, next) || contains(leftFullwidthMark, previous) || contains(fullwidthPause, previous) {
			index = end
			continue
		}
		out = append(out, chars[index:end+1]...)
		index = end
	}
	return string(out)
}
func normalizeUnprotected(text string) string {
	if text == "" || !hasNonWesternContent(text) {
		return text
	}
	var mapped strings.Builder
	mapped.Grow(len(text))
	for _, char := range text {
		if replacement, ok := safePunctuationNormalization[char]; ok {
			mapped.WriteString(replacement)
		} else {
			mapped.WriteRune(char)
		}
	}
	chars := []rune(normalizeQuotationGroups(mapped.String()))
	context := lineContext(chars)
	for index := range chars {
		if shouldFullwidth(chars, context, index) {
			chars[index] = halfToFull[chars[index]]
		}
	}
	return normalizeSpaces(string(chars))
}

// NormalizeChinesePunctuation applies SealChat conservative profile.
func NormalizeChinesePunctuation(text string) string {
	if text == "" || isMachineDocument(text) {
		return text
	}
	spans := scanProtectedSpans(text)
	if len(spans) == 0 {
		return normalizeUnprotected(text)
	}
	var result strings.Builder
	result.Grow(len(text))
	position := 0
	for _, span := range spans {
		result.WriteString(normalizeUnprotected(text[position:span.start]))
		result.WriteString(text[span.start:span.end])
		position = span.end
	}
	result.WriteString(normalizeUnprotected(text[position:]))
	return result.String()
}
