// Package set implement sets for various types. Well, only string for now :)
package set

type String struct {
	m map[string]struct{}
}

func NewString(values ...string) *String {
	s := &String{}
	s.Add(values...)
	return s
}

func (s *String) Add(values ...string) {
	if s.m == nil {
		s.m = map[string]struct{}{}
	}

	for _, v := range values {
		s.m[v] = struct{}{}
	}
}

func (s *String) Has(value string) bool {
	// We explicitly allow s to be nil *in this function* to simplify callers'
	// code.  Note that Add will not tolerate it, and will panic.
	if s == nil || s.m == nil {
		return false
	}
	_, ok := s.m[value]
	return ok
}
