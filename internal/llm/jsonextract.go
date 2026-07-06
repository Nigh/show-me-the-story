// LLM output helpers: locate the first complete JSON object in free-form
// model output (string-aware brace matching).
package llm

import "strings"

// ponytail: byte walk outside JSON strings only; truncating mid-\\ or \\u may miscount.
// Used by ExtractJSON for string-aware object boundary detection.
func WalkJSONStructure(s string, onStruct func(i int, c byte)) {
	inString := false
	escaped := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' && inString {
			escaped = true
			continue
		}
		if c == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		onStruct(i, c)
	}
}

func ExtractJSON(content string) string {
	start := strings.Index(content, "{")
	if start == -1 {
		return ""
	}
	sub := content[start:]
	depth := 0
	end := -1
	WalkJSONStructure(sub, func(i int, c byte) {
		if c == '{' {
			depth++
		} else if c == '}' {
			depth--
			if depth == 0 {
				end = i + 1
			}
		}
	})
	if end == -1 {
		return ""
	}
	return content[start : start+end]
}
