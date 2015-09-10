package statesman

import (
	"net/http"
	"testing"
	"time"
)

func TestGenerateUniqueString_returnsLongString(t *testing.T) {
	str := generateUniqueString(32)
	if len(str) < 64 {
		t.Fatalf("generateUniqueString() results too short %v", len(str))
	}
}

func TestGenerateUniqueString_returnsUniqueString(t *testing.T) {
	str0 := generateUniqueString(32)
	str1 := generateUniqueString(32)
	if str0 == str1 {
		t.Fatalf("generateUniqueString() should return unique strings %v == %v", str0, str1)
	}
}

func TestGenerateSessionCookie_ReturnsCookie(t *testing.T) {
	cookie := generateSessionCookie("name", "value", 10*time.Second)

	if cookie == nil {
		t.Fatalf("Didn't get the expected cookie\n")
	}
}

func TestFindSessionCookie_findsMatchingCookie(t *testing.T) {
	nonMatchingCookie := &http.Cookie{Name: "non matching stuff"}
	matchingCookie := &http.Cookie{Name: statesmanPrefix + "stuff"}

	c, _ := findSessionCookie([]*http.Cookie{nonMatchingCookie, matchingCookie})

	if c != matchingCookie {
		t.Fatalf("Expecting to find a matching cookie but didn't")
	}
}

func TestFindSessionCookie_returnsErrorIfNoMatchingCookie(t *testing.T) {
	nonMatchingCookie := &http.Cookie{Name: "non matching stuff"}

	_, err := findSessionCookie([]*http.Cookie{nonMatchingCookie})

	if err == nil {
		t.Fatalf("Expecting to get an error but didn't")
	}
}
