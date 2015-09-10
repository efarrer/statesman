package statesman

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"
)

// Generate a unique string
func generateUniqueString(size int) string {
	b := make([]byte, size)
	_, err := rand.Read(b)
	if err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func generateSessionCookie(name string, value string, maxage time.Duration) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Expires:  time.Now().Add(maxage),
		Secure:   false,
		HttpOnly: true,
	}
}

func findSessionCookie(cookies []*http.Cookie) (*http.Cookie, error) {
	for _, c := range cookies {
		if strings.HasPrefix(c.Name, statesmanPrefix) {
			return c, nil
		}
	}

	return nil, errors.New("Unable to find session key")
}
