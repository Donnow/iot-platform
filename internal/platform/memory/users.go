package memory

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"iot-perform/internal/platform/domain"
)

func (s *Store) GetUserByUsername(ctx context.Context, username string) (domain.User, error) {
	if err := contextErr(ctx); err != nil {
		return domain.User{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	user, exists := s.users[username]
	if !exists {
		return domain.User{}, ErrNotFound
	}
	return cloneUser(user), nil
}

func (s *Store) CreateUser(ctx context.Context, user domain.User) (domain.User, error) {
	if err := contextErr(ctx); err != nil {
		return domain.User{}, err
	}
	if user.Username == "" || user.PasswordHash == "" {
		return domain.User{}, errors.New("username and password hash are required")
	}
	if user.Role == "" {
		user.Role = "admin"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.users[user.Username]; exists {
		return domain.User{}, ErrConflict
	}
	id := fmt.Sprintf("user-%08d", atomic.AddUint64(&s.sequence, 1))
	user.ID = id
	user.CreatedAt = time.Now().UTC()
	s.users[user.Username] = user
	return cloneUser(user), nil
}

func cloneUser(user domain.User) domain.User {
	return user
}
