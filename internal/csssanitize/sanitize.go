package csssanitize

import (
	"regexp"
	"strings"
)

var (
	atImportRE = regexp.MustCompile(`(?i)@import\s+(?:url\([^)]*\)|'[^']*'|"[^"]*")[^;]*;?`)
	urlRE      = regexp.MustCompile(`(?i)url\(\s*([^)]*?)\s*\)`)
)

// Sanitize strips external resource loading from raw CSS (mirrors web cssSanitizer).
// Blocks @import and external url(); allows data:, same-origin paths, fragments.
func Sanitize(css string) string {
	result := atImportRE.ReplaceAllString(css, "/* [blocked @import] */")
	result = urlRE.ReplaceAllStringFunc(result, func(match string) string {
		sub := urlRE.FindStringSubmatch(match)
		if len(sub) < 2 {
			return "/* [blocked external url] */"
		}
		if isSafeURL(sub[1]) {
			return match
		}
		return "/* [blocked external url] */"
	})
	return result
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
			return i == 0
		}
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '+' || r == '.' || r == '-') {
			break
		}
	}
	return true
}
