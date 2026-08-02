package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"iot-perform/internal/platform/domain"
	"iot-perform/internal/platform/memory"
)

func TestIssueTokenAuthorizes(t *testing.T) {
	secret := []byte("test-secret-with-enough-length")
	token, err := IssueToken(secret, "admin", "admin", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "/api/products", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	authorizer := JWTAuthorizer{Secret: secret}
	if err := authorizer.Authorize(request); err != nil {
		t.Fatalf("token issued by IssueToken should authorize: %v", err)
	}
}

func TestIssueTokenExpires(t *testing.T) {
	secret := []byte("test-secret-with-enough-length")
	token, err := IssueToken(secret, "admin", "admin", -time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "/api/products", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	authorizer := JWTAuthorizer{Secret: secret}
	if err := authorizer.Authorize(request); err == nil {
		t.Fatal("expired token should be rejected")
	}
}

func TestLoginSuccessAndTokenWorks(t *testing.T) {
	store := memory.New()
	hash, err := HashPassword("admin123456")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateUser(t.Context(), domain.User{Username: "admin", PasswordHash: hash, Role: "admin"}); err != nil {
		t.Fatal(err)
	}
	server := NewServer(store.Repositories(), nil, nil)
	server.JWTSecret = []byte("test-secret-with-enough-length")
	server.JWTTTL = time.Hour

	login := httptest.NewRequest("POST", "/api/auth/login",
		strings.NewReader(`{"username":"admin","password":"admin123456"}`))
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, login)
	if recorder.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Token     string `json:"token"`
		ExpiresIn int    `json:"expires_in"`
		Role      string `json:"role"`
		Username  string `json:"username"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Token == "" || body.Role != "admin" || body.Username != "admin" || body.ExpiresIn != 3600 {
		t.Fatalf("unexpected login body: %+v", body)
	}

	// The issued token must work against a protected endpoint.
	products := httptest.NewRequest("GET", "/api/products", nil)
	products.Header.Set("Authorization", "Bearer "+body.Token)
	productsRecorder := httptest.NewRecorder()
	server.ServeHTTP(productsRecorder, products)
	if productsRecorder.Code != http.StatusOK {
		t.Fatalf("token from login should authorize /api/products: %d", productsRecorder.Code)
	}
}

func TestLoginRejectsBadCredentials(t *testing.T) {
	store := memory.New()
	hash, err := HashPassword("admin123456")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateUser(t.Context(), domain.User{Username: "admin", PasswordHash: hash, Role: "admin"}); err != nil {
		t.Fatal(err)
	}
	server := NewServer(store.Repositories(), nil, nil)
	server.JWTSecret = []byte("test-secret-with-enough-length")
	server.JWTTTL = time.Hour

	// Wrong password and unknown user must produce identical responses
	// (no username enumeration).
	wrongPassword := httptest.NewRequest("POST", "/api/auth/login",
		strings.NewReader(`{"username":"admin","password":"wrong-pass"}`))
	unknownUser := httptest.NewRequest("POST", "/api/auth/login",
		strings.NewReader(`{"username":"nobody","password":"whatever123"}`))
	var bodies []string
	for _, request := range []*http.Request{wrongPassword, unknownUser} {
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %s", recorder.Code, recorder.Body.String())
		}
		bodies = append(bodies, recorder.Body.String())
	}
	if bodies[0] != bodies[1] {
		t.Fatalf("credential failure responses differ (username enumeration): %q vs %q", bodies[0], bodies[1])
	}
}

func TestLoginNotConfigured(t *testing.T) {
	store := memory.New()
	hash, err := HashPassword("admin123456")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateUser(t.Context(), domain.User{Username: "admin", PasswordHash: hash, Role: "admin"}); err != nil {
		t.Fatal(err)
	}
	server := NewServer(store.Repositories(), nil, nil) // JWTSecret/JWTTTL unset

	login := httptest.NewRequest("POST", "/api/auth/login",
		strings.NewReader(`{"username":"admin","password":"admin123456"}`))
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, login)
	if recorder.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 when login is not configured, got %d", recorder.Code)
	}
}

func TestHashPasswordRejectsShortPasswords(t *testing.T) {
	if _, err := HashPassword("short"); err == nil {
		t.Fatal("short password should be rejected")
	}
}
