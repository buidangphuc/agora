package edge

import "log/slog"

func (e *Edge) principalLogAttrs(p resolvedPrincipal) []any {
	kind := p.ptype
	switch kind {
	case "user", "service":
	default:
		kind = "anonymous"
	}
	if kind == "anonymous" || p.id == "" || e.fingerprinter == nil {
		return []any{slog.String("principal_kind", kind)}
	}
	return []any{
		slog.String("principal_fp", e.fingerprinter.Of([]byte(p.id))),
		slog.String("principal_kind", kind),
	}
}
