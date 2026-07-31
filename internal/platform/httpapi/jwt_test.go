package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestJWTAuthorizerValidAndExpired(t *testing.T) {
	now := time.Unix(1000, 0).UTC()
	secret := []byte("test-secret-with-enough-length")
	authorizer := JWTAuthorizer{Secret: secret, Now: func() time.Time { return now }}
	valid := signTestJWT(secret, `{"alg":"HS256","typ":"JWT"}`, `{"sub":"operator","exp":1100}`)
	request := httptest.NewRequest("GET", "/api/products", nil)
	request.Header.Set("Authorization", "Bearer "+valid)
	if err := authorizer.Authorize(request); err != nil {
		t.Fatal(err)
	}
	expired := signTestJWT(secret, `{"alg":"HS256"}`, `{"exp":999}`)
	request.Header.Set("Authorization", "Bearer "+expired)
	if err := authorizer.Authorize(request); err == nil {
		t.Fatal("expired JWT should be rejected")
	}
}

func signTestJWT(secret []byte, header, claims string) string {
	encode := base64.RawURLEncoding.EncodeToString
	message := encode([]byte(header)) + "." + encode([]byte(claims))
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(message))
	return message + "." + encode(mac.Sum(nil))
}

func TestJWTAuthorizerRejectsWrongAlgorithm(t *testing.T) {
	authorizer := JWTAuthorizer{Secret: []byte("test-secret-with-enough-length")}
	request := httptest.NewRequest("GET", "/api/products", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 20)+"."+strings.Repeat("b", 20)+"."+strings.Repeat("c", 20))
	if err := authorizer.Authorize(request); err == nil {
		t.Fatal("malformed JWT should be rejected")
	}
}
