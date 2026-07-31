package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type JWTAuthorizer struct {
	Secret []byte
	Now    func() time.Time
}

func (a JWTAuthorizer) Authorize(request *http.Request) error {
	const prefix = "Bearer "
	header := request.Header.Get("Authorization")
	if !strings.HasPrefix(header, prefix) {
		return errors.New("bearer token is required")
	}
	if len(a.Secret) == 0 {
		return errors.New("JWT secret is required")
	}
	parts := strings.Split(strings.TrimSpace(strings.TrimPrefix(header, prefix)), ".")
	if len(parts) != 3 {
		return errors.New("invalid JWT")
	}
	decode := func(value string, target any) error {
		data, err := base64.RawURLEncoding.DecodeString(value)
		if err != nil {
			return err
		}
		return json.Unmarshal(data, target)
	}
	var headerPayload struct {
		Algorithm string `json:"alg"`
	}
	if err := decode(parts[0], &headerPayload); err != nil || headerPayload.Algorithm != "HS256" {
		return errors.New("JWT must use HS256")
	}
	var claims struct {
		ExpiresAt float64 `json:"exp"`
		NotBefore float64 `json:"nbf"`
	}
	if err := decode(parts[1], &claims); err != nil {
		return fmt.Errorf("invalid JWT claims: %w", err)
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return errors.New("invalid JWT signature")
	}
	mac := hmac.New(sha256.New, a.Secret)
	_, _ = mac.Write([]byte(parts[0] + "." + parts[1]))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return errors.New("invalid JWT signature")
	}
	now := time.Now
	if a.Now != nil {
		now = a.Now
	}
	current := float64(now().Unix())
	if claims.ExpiresAt > 0 && current >= claims.ExpiresAt {
		return errors.New("JWT has expired")
	}
	if claims.NotBefore > 0 && current < claims.NotBefore {
		return errors.New("JWT is not active")
	}
	return nil
}
