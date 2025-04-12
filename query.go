package main

import "regexp"

type query struct {
	s string
	r *regexp.Regexp
}

func (q *query) match(ref string) bool {
	if q.r != nil {
		return q.r.Match([]byte(ref))
	}
	return q.s == ref
}

func NewQueryString(ref string) *query {
	return &query{
		s: ref,
	}
}

func NewQueryRegexp(ref string) (*query, error) {
	regex, err := regexp.Compile(ref)
	if err != nil {
		return nil, err
	}
	return &query{
		r: regex,
	}, nil
}

func matchQueries(queries []*query, ref string) bool {
	for _, q := range queries {
		if q.match(ref) {
			return true
		}
	}
	return false
}
