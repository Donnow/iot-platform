package config

import "testing"

func TestDefaultConfigIsValid(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestConfigRejectsShortJWTSecret(t *testing.T) {
	c := Default()
	c.JWTSecret = "short"
	if err := c.Validate(); err == nil {
		t.Fatal("short JWT secret should be rejected")
	}
}
