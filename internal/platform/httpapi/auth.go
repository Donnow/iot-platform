package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

const defaultPasswordCost = bcrypt.DefaultCost

// HashPassword returns a bcrypt hash suitable for storage in users.password_hash.
func HashPassword(password string) (string, error) {
	if len(password) < 8 {
		return "", errors.New("password must be at least 8 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), defaultPasswordCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// handleLogin authenticates a platform user and returns a JWT.
// The endpoint sits outside the authorizer (no token required).
// Credential failures return a uniform 401 to avoid username enumeration.
func (s *Server) handleLogin(writer http.ResponseWriter, request *http.Request) {
	if s.repos.Users == nil {
		writeError(writer, http.StatusNotImplemented, errors.New("user repository is not configured"))
		return
	}
	if len(s.JWTSecret) == 0 || s.JWTTTL <= 0 {
		writeError(writer, http.StatusNotImplemented, errors.New("login is not configured"))
		return
	}
	var input loginRequest
	if err := readJSON(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	username := strings.TrimSpace(input.Username)
	if username == "" || input.Password == "" {
		writeError(writer, http.StatusBadRequest, errors.New("username and password are required"))
		return
	}
	user, err := s.repos.Users.GetUserByUsername(request.Context(), username)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Password)) != nil {
		writeError(writer, http.StatusUnauthorized, errors.New("invalid credentials"))
		return
	}
	token, err := IssueToken(s.JWTSecret, user.Username, user.Role, s.JWTTTL)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"token":      token,
		"expires_in": int(s.JWTTTL.Seconds()),
		"role":       user.Role,
		"username":   user.Username,
	})
}
