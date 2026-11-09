package service

import (
	"encoding/json"
	"html"
	"regexp"
	"strconv"
	"strings"
)

// Additional marks exist only in the LOG projection. Other AgentRich exports keep
// their existing AST and output; no HTML from the message is passed through here.
func logRichMarksFromTipTap(marks []*tiptapMark) []AgentRichMark {
	var out []AgentRichMark
	style := map[string]string{}
	var ruby *tiptapMark
	for _, mark := range marks {
		if mark == nil {
			continue
		}
		switch mark.Type {
		case "textStyle":
			for key := range mark.Attrs {
				style[key] = mark.attrString(key)
			}
		case "highlight":
			style["backgroundColor"] = firstNonEmpty(mark.attrString("color"), "#fef08a")
		case "ruby":
			ruby = mark
		default:
			out = append(out, richMarksFromTipTap([]*tiptapMark{mark})...)
		}
	}
	if len(style) > 0 {
		data, _ := json.Marshal(style)
		// Local styles sit inside basic marks (especially code), outside Ruby.
		out = append(out, AgentRichMark{Type: "logTextStyle", Value: string(data)})
	}
	if ruby != nil && strings.TrimSpace(ruby.attrString("rubyText")) != "" {
		values := map[string]string{}
		for key := range ruby.Attrs {
			values[key] = ruby.attrString(key)
		}
		data, _ := json.Marshal(values)
		out = append(out, AgentRichMark{Type: "logRuby", Value: string(data)})
	}
	return out
}

var (
	logInlineFontID   = regexp.MustCompile(`^[A-Za-z0-9_-]{1,100}$`)
	logInlineFamily   = regexp.MustCompile(`^[\p{L}\p{N}\s_,"'-]+$`)
	logInlineSize     = regexp.MustCompile(`^(\d+(?:\.\d+)?)(px|pt|em|rem|%)$`)
	logInlineColor    = regexp.MustCompile(`(?i)^(?:#[0-9a-f]{3,8}|[a-z]+|(?:rgb|rgba|hsl|hsla)\([\d\s.,%+/-]+\))$`)
	logInlineWeight   = regexp.MustCompile(`^[1-9]00$`)
	logCSSWideKeyword = regexp.MustCompile(`(?i)^(?:inherit|initial|unset|revert(?:-layer)?)$`)
)

func logStaticCSSValue(property, value string) bool {
	if value == "" || len(value) > 512 || strings.ContainsAny(value, "\\;{}<>!\x00\r\n") || strings.Contains(value, "/*") {
		return false
	}
	switch property {
	case "font-family":
		return logInlineFamily.MatchString(value) && !logCSSWideKeyword.MatchString(value)
	case "font-size":
		match := logInlineSize.FindStringSubmatch(value)
		if match == nil {
			return false
		}
		size, _ := strconv.ParseFloat(match[1], 64)
		limit := 200.0
		if match[2] == "em" || match[2] == "rem" {
			limit = 20
		} else if match[2] == "%" {
			limit = 1000
		}
		return size > 0 && size <= limit
	case "color", "background-color":
		return logInlineColor.MatchString(value) && !logCSSWideKeyword.MatchString(value)
	case "font-weight":
		return value == "normal" || value == "bold" || logInlineWeight.MatchString(value)
	case "font-style":
		return value == "normal" || value == "italic" || value == "oblique"
	case "text-decoration":
		return value == "none" || value == "underline" || value == "line-through" || value == "underline line-through"
	}
	return false
}

func logRichBlockAttributes(node AgentRichNode, enabled bool) string {
	if enabled {
		switch value := node.Attrs["textAlign"]; value {
		case "left", "center", "right", "justify", "start", "end":
			return ` style="text-align: ` + value + `"`
		}
	}
	return ""
}

// A closed mapping retains only static text values and platform font identities.
// The browser applies the same body whitelist again before any template insertion.
func logRichInlineAttributes(values map[string]string, ruby bool) string {
	var styles, attrs, classes []string
	if ruby {
		classes = append(classes, "tiptap-ruby")
	}
	fields := []struct{ key, property string }{
		{"fontFamily", "font-family"}, {"fontSize", "font-size"}, {"color", "color"}, {"backgroundColor", "background-color"},
	}
	if ruby {
		fields = []struct{ key, property string }{
			{"rubyFontFamily", "font-family"}, {"rubyBaseFontFamily", "font-family"}, {"rubyRtFontFamily", "font-family"},
			{"rubyFontSize", "font-size"}, {"rubyBaseFontSize", "font-size"}, {"rubyRtFontSize", "font-size"}, {"rubyRtScale", "font-size"},
			{"rubyColor", "color"}, {"rubyBackgroundColor", "background-color"}, {"rubyFontWeight", "font-weight"},
			{"rubyFontStyle", "font-style"}, {"rubyTextDecoration", "text-decoration"},
		}
	}
	for _, field := range fields {
		value := strings.TrimSpace(values[field.key])
		if !logStaticCSSValue(field.property, value) {
			continue
		}
		property := field.property
		if ruby {
			var name strings.Builder
			for _, char := range field.key {
				if char >= 'A' && char <= 'Z' {
					name.WriteByte('-')
					char += 'a' - 'A'
				}
				name.WriteRune(char)
			}
			property = "--" + name.String()
		}
		styles = append(styles, property+": "+value)
	}
	fontFields := []struct{ key, attr string }{{"fontAssetId", "data-platform-font-id"}}
	if ruby {
		fontFields = []struct{ key, attr string }{{"rubyFontAssetId", "data-platform-font-id"}, {"rubyBaseFontAssetId", "data-ruby-base-font-asset-id"}, {"rubyRtFontAssetId", "data-ruby-rt-font-asset-id"}}
	}
	for _, field := range fontFields {
		if id := values[field.key]; logInlineFontID.MatchString(id) {
			attrs = append(attrs, field.attr+`="`+id+`"`)
		}
	}
	if !ruby {
		line := values["textDecorationLine"]
		if line == "underline" || line == "line-through" {
			kind := "underline"
			if line == "line-through" {
				kind = "strike"
			}
			choose := func(value, fallback string, allowed ...string) string {
				for _, item := range allowed {
					if value == item {
						return value
					}
				}
				return fallback
			}
			classes = append(classes, "tiptap-text-decoration", "tiptap-text-decoration--"+kind,
				"tiptap-text-decoration--"+choose(values["textDecorationPattern"], "solid", "solid", "dotted", "dense-dotted"),
				"tiptap-text-decoration--wave-"+choose(values["textDecorationWave"], "none", "none", "soft", "heavy"),
				"tiptap-text-decoration--"+choose(values["textDecorationCount"], "single", "single", "double", "triple"))
			thickness := map[string]string{"thin": "1px", "regular": "0.12em", "bold": "0.18em"}[values["textDecorationThickness"]]
			styles = append(styles, "--tiptap-decoration-thickness: "+firstNonEmpty(thickness, "0.12em"))
		}
	}
	if len(classes) > 0 {
		attrs = append(attrs, `class="`+strings.Join(classes, " ")+`"`)
	}
	if len(styles) > 0 {
		attrs = append(attrs, `style="`+html.EscapeString(strings.Join(styles, "; "))+`"`)
	}
	if len(attrs) == 0 {
		return ""
	}
	return " " + strings.Join(attrs, " ")
}

func wrapLogHTMLMarks(value string, marks []AgentRichMark) string {
	for index := len(marks) - 1; index >= 0; index-- {
		mark := marks[index]
		switch mark.Type {
		case "logTextStyle", "logRuby":
			values := map[string]string{}
			if json.Unmarshal([]byte(mark.Value), &values) != nil {
				continue
			}
			if mark.Type == "logRuby" {
				value = "<ruby" + logRichInlineAttributes(values, true) + ">" + value + "<rt>" + html.EscapeString(strings.TrimSpace(values["rubyText"])) + "</rt></ruby>"
			} else if attrs := logRichInlineAttributes(values, false); attrs != "" {
				tag := "span"
				if values["backgroundColor"] != "" {
					tag = "mark"
				}
				value = "<" + tag + attrs + ">" + value + "</" + tag + ">"
			}
		default:
			value = wrapAgentHTMLMarks(value, []AgentRichMark{mark})
		}
	}
	return value
}
