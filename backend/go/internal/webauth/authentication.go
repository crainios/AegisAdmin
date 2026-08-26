package webauth

import (
	"context"
	"strings"

	"aegisadmin/backend/internal/authstore"
)

type UserRepository interface {
	FindByLogin(context.Context, string) (authstore.User, bool, error)
	FindRoot(context.Context) (authstore.User, bool, error)
}

type NextStep string

const (
	StepInvalid             NextStep = "invalid"
	StepAuthenticated       NextStep = "authenticated"
	StepPasswordChange      NextStep = "password_change"
	StepTwoFactorChallenge  NextStep = "two_factor_challenge"
	StepTwoFactorEnrollment NextStep = "two_factor_enrollment"
)

type Result struct {
	User authstore.User
	Step NextStep
}

type Authenticator struct {
	users UserRepository
}

func New(users UserRepository) *Authenticator {
	return &Authenticator{users: users}
}

func (a *Authenticator) Authenticate(ctx context.Context, login, password string) (Result, error) {
	login = strings.TrimSpace(login)
	user, found, err := a.users.FindByLogin(ctx, login)
	if err != nil {
		return Result{}, err
	}
	reference := user
	if !found {
		reference, _, err = a.users.FindRoot(ctx)
		if err != nil {
			return Result{}, err
		}
	}
	passwordValid := VerifyPassword(password, reference.PasswordHash)
	if !found || !passwordValid || user.Status != "active" {
		return Result{Step: StepInvalid}, nil
	}
	if user.TOTPSecret.Valid && user.TOTPEnabledAt.Valid {
		return Result{User: user, Step: StepTwoFactorChallenge}, nil
	}
	if user.TwoFactorRequired {
		return Result{User: user, Step: StepTwoFactorEnrollment}, nil
	}
	if user.MustChangePassword {
		return Result{User: user, Step: StepPasswordChange}, nil
	}
	return Result{User: user, Step: StepAuthenticated}, nil
}
