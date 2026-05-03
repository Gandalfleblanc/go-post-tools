package main

// withRetry : retry exponentiel pour les appels Hydracker (gère 522 Cloudflare,
// timeouts réseau, 5xx). Pas de retry sur les erreurs logiques (4xx, parsing).

import (
	"log"
	"strings"
	"time"
)

func withRetry(name string, attempts int, fn func() error) error {
	var err error
	for i := 1; i <= attempts; i++ {
		err = fn()
		if err == nil {
			return nil
		}
		if !isRetryableErr(err) {
			return err // erreur logique : pas de retry
		}
		if i == attempts {
			break
		}
		backoff := time.Duration(1<<i) * time.Second // 2s, 4s, 8s, ...
		log.Printf("[retry] %s attempt %d/%d failed: %v (retry in %s)", name, i, attempts, err, backoff)
		time.Sleep(backoff)
	}
	return err
}

func isRetryableErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	retryable := []string{
		"522", "503", "504", "502",
		"timeout", "timed out",
		"connection refused", "connection reset",
		"eof",
		"no such host",
		"i/o timeout",
		"network is unreachable",
		"temporarily unavailable",
	}
	for _, p := range retryable {
		if strings.Contains(msg, p) {
			return true
		}
	}
	return false
}
