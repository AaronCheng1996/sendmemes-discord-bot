package persistent

import "strings"

// escapeILikePattern builds a PostgreSQL ILIKE pattern with wildcards,
// escaping \, %, and _ inside the user's input.
func escapeILikePattern(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	raw = strings.ReplaceAll(raw, `\`, `\\`)
	raw = strings.ReplaceAll(raw, `%`, `\%`)
	raw = strings.ReplaceAll(raw, `_`, `\_`)
	return "%" + raw + "%"
}
