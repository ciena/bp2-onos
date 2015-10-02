package onos

import "sort"

// StringSet a minimal and cheap implementation of a string set capability
type StringSet map[string]interface{}

// Add adds a string to the set
func (s StringSet) Add(v string) {
	s[v] = nil
}

// Remove removes a string from the set
func (s StringSet) Remove(v string) {
	delete(s, v)
}

// Array returns an array of the set
func (s StringSet) Array() []string {
	keys := make([]string, 0, len(s))
	for k := range s {
		keys = append(keys, k)
	}
	return keys
}

// Equals true if sets are equal, else false
func (s StringSet) Equals(t StringSet) bool {
	if len(s) != len(t) {
		return false
	}

	k1 := s.Array()
	k2 := t.Array()

	sort.Sort(AlphaOrder(k1))
	sort.Sort(AlphaOrder(k2))

	for i := range k1 {
		if k1[i] != k2[i] {
			return false
		}
	}
	return true
}

// Contains trye if set contains string, else false
func (s StringSet) Contains(v string) bool {
	_, ok := s[v]
	return ok
}
