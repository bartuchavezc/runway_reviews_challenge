package env

import (
	"log"
	"os"
	"strconv"
)

func GetString(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

// GetInt returns the integer value of key, or fallback if the variable is not set.
// If the variable IS set but cannot be parsed as an integer the server exits immediately —
// a misconfigured environment value (e.g. POLL_INTERVAL_MINUTES=abc) is a programming
// error and should never be silently ignored.
func GetInt(key string, fallback int) int {
	v, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		log.Fatalf("[package:env][message:invalid value for %s: %q — must be an integer; fix the environment and restart]", key, v)
	}
	return n
}
