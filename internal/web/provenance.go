package web

// provenanceView is what a template needs to draw the marker beside one control:
// the variable's name, the two values, and whether they are in conflict.
//
// A view type rather than shared.Provenance directly, because the template must
// not have to call a method to know whether to draw anything — nil is the whole
// guard, and {{with}} reads it.
type provenanceView struct {
	// Variable is the environment variable's name, shown so the operator knows
	// where to go and change it.
	Variable string
	// Env is what that variable says.
	Env string
	// Stored is the operator's own value, empty when they have not set one.
	Stored string
	// Overridden is true only when the two differ. A stored value identical to
	// the variable's is not a conflict, and offering to "reset" it would invite
	// an operator to undo something that changes nothing.
	Overridden bool
}

// provenanceFor returns the marker for one TOML key, or nil when no environment
// variable names it — which is every key on an installation configured the
// ordinary way, and why the marker is absent rather than empty there.
func (s *Server) provenanceFor(key string) *provenanceView {
	p, ok := s.cfg.Provenance(key)
	if !ok {
		return nil
	}
	return &provenanceView{
		Variable:   p.Name,
		Env:        p.Env,
		Stored:     p.Stored,
		Overridden: p.Overridden(),
	}
}
