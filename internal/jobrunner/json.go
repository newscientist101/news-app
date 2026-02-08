package jobrunner

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	// Regex to find JSON array in text
	jsonArrayRegex = regexp.MustCompile(`(?s)\[.*\]`)

	// Regex to remove markdown code blocks
	codeBlockStart = regexp.MustCompile("(?m)^\\s*```(?:json)?\\s*")
	codeBlockEnd   = regexp.MustCompile("(?m)\\s*```\\s*")
)

// extractJSONArray finds and extracts a JSON array from text.
func extractJSONArray(text string) (string, error) {
	// Remove markdown code blocks
	text = codeBlockStart.ReplaceAllString(text, "")
	text = codeBlockEnd.ReplaceAllString(text, "")
	text = strings.TrimSpace(text)

	// Find JSON array
	match := jsonArrayRegex.FindString(text)
	if match == "" {
		return "", fmt.Errorf("no JSON array found in response")
	}

	return match, nil
}

// fixMalformedJSON attempts to fix common LLM JSON issues.
// This handles unescaped quotes inside strings (common with Chinese text).
func fixMalformedJSON(s string) string {
	var result strings.Builder
	result.Grow(len(s))

	inString := false
	escapeNext := false

	for i := 0; i < len(s); i++ {
		c := s[i]

		if escapeNext {
			result.WriteByte(c)
			escapeNext = false
			continue
		}

		if c == '\\' {
			result.WriteByte(c)
			escapeNext = true
			continue
		}

		if c == '"' {
			if !inString {
				inString = true
				result.WriteByte(c)
			} else {
				// Check if this looks like end of string
				rest := ""
				if i+1 < len(s) {
					endIdx := i + 20
					if endIdx > len(s) {
						endIdx = len(s)
					}
					rest = strings.TrimLeft(s[i+1:endIdx], " \t\n\r")
				}

				if rest == "" || rest[0] == ',' || rest[0] == '}' || rest[0] == ']' || rest[0] == ':' {
					// End of string
					inString = false
					result.WriteByte(c)
				} else {
					// Embedded quote - escape it
					result.WriteString("\\\"")
				}
			}
		} else {
			result.WriteByte(c)
		}
	}

	return result.String()
}
