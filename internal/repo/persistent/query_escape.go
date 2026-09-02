package persistent

import "strings"

// escapeILikePattern builds a PostgreSQL ILIKE pattern with wildcards,
// escaping \, %, and _ inside the user's input.
func escapeILikePattern(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	return "%" + escapeLikePrefix(raw) + "%"
}

// escapeLikePrefix escapes \, %, and _ in a literal that will be embedded in a
// LIKE pattern, WITHOUT adding wildcards. Callers that anchor the match
// themselves (a path prefix, say) append their own.
func escapeLikePrefix(raw string) string {
	raw = strings.ReplaceAll(raw, `\`, `\\`)
	raw = strings.ReplaceAll(raw, `%`, `\%`)
	raw = strings.ReplaceAll(raw, `_`, `\_`)
	return raw
}
