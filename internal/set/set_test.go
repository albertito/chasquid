package set

import "testing"

func TestString(t *testing.T) {
	s1 := &String{}

	// Test that Has works on a new set.
	if s1.Has("x") {
		t.Error("'x' is in the empty set")
	}

	s1.Add("a")
	s1.Add("b", "ccc")

	expectStrings(s1, []string{"a", "b", "ccc"}, []string{"not-in"}, t)

	s2 := NewString("a", "b", "c")
	expectStrings(s2, []string{"a", "b", "c"}, []string{"not-in"}, t)

	// Test that Has works (and not panics) on a nil set.
	var s3 *String
	if s3.Has("x") {
		t.Error("'x' is in the nil set")
	}
}

func expectStrings(s *String, in []string, notIn []string, t *testing.T) {
	for _, str := range in {
		if !s.Has(str) {
			t.Errorf("String %q not in set, it should be", str)
		}
	}

	for _, str := range notIn {
		if s.Has(str) {
			t.Errorf("String %q is in the set, should not be", str)
		}
	}
}
