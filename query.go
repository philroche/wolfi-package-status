package main

type Matcher interface {
	MatchString(s string) bool
}

type queryString struct {
	s string
}

func (q *queryString) MatchString(ref string) bool {
	return q.s == ref
}

func NewQueryString(ref string) Matcher {
	return &queryString{
		s: ref,
	}
}

func matchReference(queries []Matcher, ref string) bool {
	for _, q := range queries {
		if q.MatchString(ref) {
			return true
		}
	}
	return false
}
