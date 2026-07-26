package csssanitize

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var (
	atImportRE = regexp.MustCompile(`(?i)@import\s+(?:url\([^)]*\)|'[^']*'|"[^"]*")[^;]*;?`)
	urlRE      = regexp.MustCompile(`(?i)url\(\s*([^)]*?)\s*\)`)
)

// Sanitize strips external resource loading from raw CSS (mirrors web cssSanitizer).
// Blocks @import and external url(); allows data:, same-origin paths, fragments.
// Comments are removed and CSS escapes are normalized before matching so forms
// like `@import/**/ "https://…"` and `url(h\74 tps://…)` cannot bypass checks.
func Sanitize(css string) string {
	normalized := stripCSSComments(css)
	result := atImportRE.ReplaceAllString(normalized, "/* [blocked @import] */")
	result = urlRE.ReplaceAllStringFunc(result, func(match string) string {
		sub := urlRE.FindStringSubmatch(match)
		if len(sub) < 2 {
			return "/* [blocked external url] */"
		}
		if isSafeURL(unescapeCSS(sub[1])) {
			return match
		}
		return "/* [blocked external url] */"
	})
	return result
}

func stripCSSComments(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if i+1 < len(s) && s[i] == '/' && s[i+1] == '*' {
			end := strings.Index(s[i+2:], "*/")
			if end < 0 {
				break
			}
			i = i + 2 + end + 2
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// unescapeCSS resolves CSS escapes (hex and single-char) used inside url()/imports.
func unescapeCSS(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			i++
			continue
		}
		j := i + 1
		hexDigits := 0
		for j < len(s) && hexDigits < 6 && isHexByte(s[j]) {
			j++
			hexDigits++
		}
		if hexDigits > 0 {
			val, err := strconv.ParseInt(s[i+1:j], 16, 32)
			if err == nil {
				b.WriteRune(rune(val))
			}
			if j < len(s) && isCSSWhitespace(s[j]) {
				j++
			}
			i = j
			continue
		}
		b.WriteByte(s[i+1])
		i += 2
	}
	return b.String()
}

func isHexByte(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func isCSSWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

func isSafeURL(urlContent string) bool {
	trimmed := strings.TrimSpace(urlContent)
	trimmed = strings.Trim(trimmed, `"'`)
	if trimmed == "" {
		return true
	}
	if strings.HasPrefix(trimmed, "data:") {
		return true
	}
	if strings.HasPrefix(trimmed, "/") && !strings.HasPrefix(trimmed, "//") {
		return true
	}
	if strings.HasPrefix(trimmed, "#") {
		return true
	}
	if strings.HasPrefix(trimmed, "//") {
		return false
	}
	for i, r := range trimmed {
		if r == ':' {
			// Scheme present (http:, https:, javascript:, …) — reject.
			return i == 0
		}
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '+' && r != '.' && r != '-' {
			break
		}
	}
	return true
}
