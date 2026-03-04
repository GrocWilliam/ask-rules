// application/usecase/upsert_game.go — Use case pour créer/mettre à jour un jeu
package usecase

import (
	"context"
	"fmt"
	"log"
	"time"

	"ask-rules-server/internal/domain/entity"
	"ask-rules-server/internal/domain/repository"
)

// UpsertGameUseCase gère la logique métier pour créer ou mettre à jour un jeu.
type UpsertGameUseCase struct {
	gameRepo repository.GameRepository
}

// NewUpsertGameUseCase crée un nouveau use case.
func NewUpsertGameUseCase(gameRepo repository.GameRepository) *UpsertGameUseCase {
	return &UpsertGameUseCase{gameRepo: gameRepo}
}

// UpsertGameRequest représente la requête pour créer/mettre à jour un jeu.
type UpsertGameRequest struct {
	ID       string                 `json:"id"`
	Name     string                 `json:"name"`
	Gameplay map[string]interface{} `json:"gameplay,omitempty"`
}

// Execute crée ou met à jour un jeu.
func (uc *UpsertGameUseCase) Execute(ctx context.Context, req *UpsertGameRequest) (*GameDetailDTO, error) {
	// Validation
	if req.Name == "" {
		return nil, entity.ErrInvalidGameName
	}

	// Créer l'entité
	now := time.Now()
	game := &entity.Game{
		ID:       req.ID,
		Name:     req.Name,
		Gameplay: req.Gameplay,
	}

	// Si c'est une création (pas d'ID), générer un ID
	if game.ID == "" {
		game.ID = generateGameID(req.Name)
		game.AddedAt = now
	} else {
		// Si mise à jour, récupérer AddedAt existant
		existing, err := uc.gameRepo.FindByID(ctx, game.ID)
		if err == nil && existing != nil {
			game.AddedAt = existing.AddedAt
		} else {
			game.AddedAt = now
		}
	}

	// Sauvegarder
	if err := uc.gameRepo.Save(ctx, game); err != nil {
		log.Printf("[ERROR] UpsertGame - Failed to save game '%s': %v", req.Name, err)
		return nil, fmt.Errorf("failed to save game: %w", err)
	}

	// Retourner le DTO
	return &GameDetailDTO{
		ID:       game.ID,
		Name:     game.Name,
		AddedAt:  game.AddedAt,
		Gameplay: game.Gameplay,
	}, nil
}
