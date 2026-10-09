package filter

import (
	"net/url"
	"strings"
)

// ParseMentions extracts unique @mentions from content, preserving first-seen
// order. A mention is `@` followed by a word of letters, digits, _ or -.
// The scan is allocation-light: it reuses a small dedupe via linear scan for
// the common case of a handful of mentions before falling back to a map.
func ParseMentions(content string) []string {
	var out []string
	for i := 0; i < len(content); i++ {
		if content[i] != '@' {
			continue
		}
		j := i + 1
		if j >= len(content) || !isMentionStart(content[j]) {
			continue
		}
		j++
		for j < len(content) && isMentionPart(content[j]) {
			j++
		}
		name := content[i+1 : j]
		if !contains(out, name) {
			out = append(out, name)
		}
		i = j - 1
	}
	if out == nil {
		return []string{}
	}
	return out
}

func isMentionStart(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_'
}

func isMentionPart(c byte) bool {
	return isMentionStart(c) || c == '-'
}

// contains reports whether s is already in a small slice (linear scan beats a
// map allocation for the typical mention counts).
func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// CSV splits a comma separated value, trimming blanks.
func CSV(v string) []string {
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// CSVFirst returns the first present value among the given param keys.
func CSVFirst(params url.Values, keys ...string) []string {
	for _, k := range keys {
		if v, ok := params[k]; ok && len(v) > 0 {
			return CSV(v[0])
		}
	}
	return nil
}

// First returns the first value for a key, or "" when absent.
func First(params url.Values, key string) (string, bool) {
	v, ok := params[key]
	if !ok || len(v) == 0 {
		return "", false
	}
	return v[0], true
}
