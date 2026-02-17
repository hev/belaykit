package claude

import (
	"encoding/json"
	"fmt"
	"strings"
)

// StripCodeFences removes markdown code fences from a string so the
// content inside can be parsed cleanly. Handles ```json ... ``` wrapping
// and duplicated blocks that some models produce.
func StripCodeFences(s string) string {
	var result strings.Builder
	lines := strings.Split(s, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			continue
		}
		result.WriteString(line)
		result.WriteByte('\n')
	}
	return result.String()
}

// ExtractJSON strips code fences, finds the first JSON object ({...}) in
// the string, and unmarshals it into dst.
func ExtractJSON(s string, dst any) error {
	s = StripCodeFences(s)

	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start == -1 || end == -1 || end <= start {
		return ErrNoJSON
	}

	jsonStr := s[start : end+1]
	if err := json.Unmarshal([]byte(jsonStr), dst); err != nil {
		return fmt.Errorf("parsing JSON: %w", err)
	}
	return nil
}

// ExtractJSONArray strips code fences, finds the first JSON array ([...])
// in the string, and unmarshals it into dst.
func ExtractJSONArray(s string, dst any) error {
	s = StripCodeFences(s)

	start := strings.Index(s, "[")
	end := strings.LastIndex(s, "]")
	if start == -1 || end == -1 || end <= start {
		return ErrNoJSON
	}

	jsonStr := s[start : end+1]
	if err := json.Unmarshal([]byte(jsonStr), dst); err != nil {
		return fmt.Errorf("parsing JSON: %w", err)
	}
	return nil
}
