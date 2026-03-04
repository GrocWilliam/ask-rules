// application/usecase/admin_auth_test.go — Tests unitaires du use case AdminAuth
package usecase_test

import (
	"context"
	"testing"
	"time"

	"ask-rules-server/internal/application/usecase"
)

// Test : Login avec mot de passe valide
func TestAdminAuthUseCase_Login_Success(t *testing.T) {
	// Arrange
	password := "admin123"
	uc := usecase.NewAdminAuthUseCase(password, nil)

	req := &usecase.LoginRequest{
		Password: password,
	}

	// Act
	response, err := uc.Login(context.Background(), req)

	// Assert
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if response.Token == "" {
		t.Error("Expected token to be generated")
	}

	if response.Expires.Before(time.Now()) {
		t.Error("Expected expiration time to be in the future")
	}

	// Vérifier que le token est valide
	valid, err := uc.CheckSession(context.Background(), response.Token)
	if err != nil {
		t.Fatalf("Expected no error checking session, got: %v", err)
	}

	if !valid {
		t.Error("Expected token to be valid after login")
	}
}

// Test : Login avec mot de passe invalide
func TestAdminAuthUseCase_Login_InvalidPassword(t *testing.T) {
	// Arrange
	uc := usecase.NewAdminAuthUseCase("correct-password", nil)

	req := &usecase.LoginRequest{
		Password: "wrong-password",
	}

	// Act
	response, err := uc.Login(context.Background(), req)

	// Assert
	if err == nil {
		t.Fatal("Expected error for invalid password, got nil")
	}

	if err != usecase.ErrInvalidPassword {
		t.Errorf("Expected ErrInvalidPassword, got: %v", err)
	}

	if response != nil {
		t.Errorf("Expected nil response on error, got: %+v", response)
	}
}

// Test : Logout révoque la session
func TestAdminAuthUseCase_Logout_Success(t *testing.T) {
	// Arrange
	password := "admin123"
	uc := usecase.NewAdminAuthUseCase(password, nil)

	// Login d'abord
	loginResp, _ := uc.Login(context.Background(), &usecase.LoginRequest{Password: password})

	// Act
	err := uc.Logout(context.Background(), loginResp.Token)

	// Assert
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Vérifier que la session n'est plus valide
	valid, _ := uc.CheckSession(context.Background(), loginResp.Token)
	if valid {
		t.Error("Expected session to be invalid after logout")
	}
}

// Test : CheckSession avec token invalide
func TestAdminAuthUseCase_CheckSession_InvalidToken(t *testing.T) {
	// Arrange
	uc := usecase.NewAdminAuthUseCase("password", nil)

	// Act
	valid, err := uc.CheckSession(context.Background(), "invalid-token")

	// Assert
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	if valid {
		t.Error("Expected session to be invalid")
	}
}

// Test : CheckSession avec token vide
func TestAdminAuthUseCase_CheckSession_EmptyToken(t *testing.T) {
	// Arrange
	uc := usecase.NewAdminAuthUseCase("password", nil)

	// Act
	valid, err := uc.CheckSession(context.Background(), "")

	// Assert
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	if valid {
		t.Error("Expected session to be invalid")
	}
}

// Test : Plusieurs sessions simultanées
func TestAdminAuthUseCase_MultipleSessions(t *testing.T) {
	// Arrange
	password := "admin123"
	uc := usecase.NewAdminAuthUseCase(password, nil)

	// Act - Créer deux sessions
	resp1, _ := uc.Login(context.Background(), &usecase.LoginRequest{Password: password})
	resp2, _ := uc.Login(context.Background(), &usecase.LoginRequest{Password: password})

	// Assert
	if resp1.Token == resp2.Token {
		t.Error("Expected different tokens for different sessions")
	}

	// Les deux doivent être valides
	valid1, _ := uc.CheckSession(context.Background(), resp1.Token)
	valid2, _ := uc.CheckSession(context.Background(), resp2.Token)

	if !valid1 {
		t.Error("Expected first session to be valid")
	}

	if !valid2 {
		t.Error("Expected second session to be valid")
	}

	// Logout de la première session ne doit pas affecter la seconde
	uc.Logout(context.Background(), resp1.Token)

	valid1After, _ := uc.CheckSession(context.Background(), resp1.Token)
	valid2After, _ := uc.CheckSession(context.Background(), resp2.Token)

	if valid1After {
		t.Error("Expected first session to be invalid after logout")
	}

	if !valid2After {
		t.Error("Expected second session to still be valid")
	}
}
