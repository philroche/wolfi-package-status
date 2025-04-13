package main

import (
	"cmp"
	"os"
	"slices"

	"golang.org/x/exp/maps"
)

func getEnvWithFallback(envName string, fallback string) string {
	if value, exists := os.LookupEnv(envName); exists {
		return value
	}
	return fallback
}

func dedup[T cmp.Ordered](arr []T) []T {
	m := make(map[T]bool, len(arr))
	for _, v := range arr {
		m[v] = true
	}

	l := maps.Keys(m)
	slices.Sort(l)

	return l
}
