package identifier

import (
	"regexp"
	"strings"
	"unicode"
)

var multiUnderscore = regexp.MustCompile(`_+`)

// NormalizeColumnName converts source column names into snake_case while
// preserving the original lexical content as much as possible.
func NormalizeColumnName(value string) string {
	s := strings.TrimSpace(value)
	if s == "" {
		return ""
	}

	s = strings.ReplaceAll(s, "%", "pct")
	s = strings.ReplaceAll(s, "/", "_per_")
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, "(", "_")
	s = strings.ReplaceAll(s, ")", "")

	var b strings.Builder
	b.Grow(len(s) * 2)

	var prev rune
	for i, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if i > 0 && shouldInsertUnderscore(prev, r) && b.Len() > 0 {
				last := rune(b.String()[b.Len()-1])
				if last != '_' {
					b.WriteByte('_')
				}
			}
			b.WriteRune(unicode.ToLower(r))
		default:
			if b.Len() > 0 {
				last := rune(b.String()[b.Len()-1])
				if last != '_' {
					b.WriteByte('_')
				}
			}
		}
		prev = r
	}

	normalized := multiUnderscore.ReplaceAllString(b.String(), "_")
	return strings.Trim(normalized, "_")
}

func QuoteIfNeeded(identifier string) string {
	if identifier == "" {
		return `""`
	}
	if startsWithDigit(identifier) {
		return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
	}
	return identifier
}

func shouldInsertUnderscore(prev, current rune) bool {
	if prev == 0 {
		return false
	}
	if unicode.IsLower(prev) && unicode.IsUpper(current) {
		return true
	}
	if unicode.IsDigit(prev) && unicode.IsLetter(current) {
		return true
	}
	if unicode.IsLetter(prev) && unicode.IsDigit(current) {
		return true
	}
	return false
}

func startsWithDigit(value string) bool {
	for _, r := range value {
		return unicode.IsDigit(r)
	}
	return false
}
