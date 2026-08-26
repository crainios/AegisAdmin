package webauth

import (
	"context"
	"database/sql"
	"testing"

	"aegisadmin/backend/internal/authstore"
	"golang.org/x/crypto/bcrypt"
)

type fakeUsers struct {
	user  authstore.User
	found bool
	root  authstore.User
}

func (f fakeUsers) FindByLogin(context.Context, string) (authstore.User, bool, error) {
	return f.user, f.found, nil
}
func (f fakeUsers) FindRoot(context.Context) (authstore.User, bool, error) {
	return f.root, f.root.ID != 0, nil
}

func TestAuthenticationNextSteps(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("valid password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	base := authstore.User{ID: 2, Login: "user", PasswordHash: string(hash), Status: "active"}
	tests := []struct {
		name string
		user authstore.User
		step NextStep
	}{
		{"authenticated", base, StepAuthenticated},
		{"password", withPasswordChange(base), StepPasswordChange},
		{"enrollment", withRequiredTwoFactor(base), StepTwoFactorEnrollment},
		{"challenge", withEnabledTwoFactor(base), StepTwoFactorChallenge},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := New(fakeUsers{user: test.user, found: true}).Authenticate(context.Background(), "user", "valid password")
			if err != nil || result.Step != test.step {
				t.Fatalf("result = %#v, err = %v", result, err)
			}
		})
	}
}

func TestAuthenticationUsesRootHashForUnknownLogin(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("root password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	result, err := New(fakeUsers{root: authstore.User{ID: 1, PasswordHash: string(hash)}}).Authenticate(
		context.Background(), "unknown", "root password",
	)
	if err != nil || result.Step != StepInvalid {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
}

func withPasswordChange(user authstore.User) authstore.User {
	user.MustChangePassword = true
	return user
}
func withRequiredTwoFactor(user authstore.User) authstore.User {
	user.TwoFactorRequired = true
	return user
}
func withEnabledTwoFactor(user authstore.User) authstore.User {
	user.TOTPSecret = sql.NullString{String: "secret", Valid: true}
	user.TOTPEnabledAt = sql.NullString{String: "now", Valid: true}
	return user
}
