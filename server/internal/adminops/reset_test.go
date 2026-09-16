package adminops

import "testing"

func TestValidateNewPasswordRequiresTwelveCharacters(t *testing.T) {
	if err := ValidateNewPassword("short-pass"); err == nil {
		t.Fatal("expected short password to be rejected")
	}
	if err := ValidateNewPassword("long-password-2026"); err != nil {
		t.Fatal(err)
	}
}
