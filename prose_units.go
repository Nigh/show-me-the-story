package main

import "unicode"

// countProseUnits counts chapter prose length: CJK runes +1 each; contiguous
// Latin alnum tokens (with internal . , - # glue) +1 each. Punctuation and
// whitespace break tokens and are not counted. Fullwidth alnum normalizes to halfwidth.
func countProseUnits(s string) int {
	count := 0
	inToken := false
	for _, r := range s {
		r = normalizeWideAlnum(r)
		switch {
		case isCJK(r):
			if inToken {
				count++
				inToken = false
			}
			count++
		case isLatinAlnum(r):
			inToken = true
		case isTokenGlue(r):
			if inToken {
				continue
			}
		case isWhitespace(r), isBreakPunct(r):
			if inToken {
				count++
				inToken = false
			}
		default:
			if inToken {
				count++
				inToken = false
			}
		}
	}
	if inToken {
		count++
	}
	return count
}

func normalizeWideAlnum(r rune) rune {
	switch {
	case r >= '０' && r <= '９':
		return r - '０' + '0'
	case r >= 'Ａ' && r <= 'Ｚ':
		return r - 'Ａ' + 'A'
	case r >= 'ａ' && r <= 'ｚ':
		return r - 'ａ' + 'a'
	default:
		return r
	}
}

func isLatinAlnum(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
}

func isTokenGlue(r rune) bool {
	return r == '.' || r == ',' || r == '-' || r == '#'
}

func isWhitespace(r rune) bool {
	return unicode.IsSpace(r)
}

func isBreakPunct(r rune) bool {
	if isCJKPunct(r) {
		return true
	}
	return isLatinBreakPunct(r)
}

func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		unicode.Is(unicode.Hiragana, r) ||
		unicode.Is(unicode.Katakana, r) ||
		unicode.Is(unicode.Hangul, r)
}

func isCJKPunct(r rune) bool {
	switch r {
	case '。', '，', '、', '；', '：', '！', '？', '…', '—', '「', '」', '『', '』', '（', '）', '《', '》', '【', '】', '“', '”', '‘', '’', '·':
		return true
	}
	return false
}

func isLatinBreakPunct(r rune) bool {
	switch r {
	case '.', ',', ';', ':', '!', '?', '(', ')', '[', ']', '{', '}', '"', '\'', '/', '\\', '@', '$', '%', '^', '&', '*', '+', '=', '|', '~', '`', '<', '>':
		return true
	}
	return false
}
