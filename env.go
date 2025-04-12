package main

import "os"

func getEnvWithFallback(envName string, fallback string) string {
	if value, exists := os.LookupEnv(envName); exists {
		return value
	}
	return fallback
}

