package filter

import (
	"net/url"
	"regexp"
	"strings"
)

// mentionRe matches @mentions: @ followed by a word of letters, digits, _ or -.
var mentionRe = regexp.MustCompile(`@([A-Za-z0-9_][A-Za-z0-9_-]*)`)

// ParseMentions extracts unique @mentions from content, preserving first-seen order.
func ParseMentions(content string) []string {
	matches := mentionRe.FindAllStringSubmatch(content, -1)
	out := make([]string, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))
	for _, m := range matches {
		if _, ok := seen[m[1]]; ok {
			continue
		}
		seen[m[1]] = struct{}{}
		out = append(out, m[1])
	}
	return out
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
