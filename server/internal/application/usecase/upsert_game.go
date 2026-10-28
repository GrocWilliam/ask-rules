// application/usecase/upsert_game.go — Use case pour créer/mettre à jour un jeu
package usecase

import (
	"context"
	"errors"
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

	id := req.ID
	if id == "" {
		id = generateGameID(req.Name)
	}

	// Mise à jour : partir du jeu existant, Save réécrit toutes les colonnes
	// (sinon fichiers, métadonnées et statistiques seraient effacés)
	game, err := uc.gameRepo.FindByID(ctx, id)
	if err != nil {
		if !errors.Is(err, entity.ErrGameNotFound) {
			return nil, fmt.Errorf("failed to find game: %w", err)
		}
		game = &entity.Game{ID: id, AddedAt: time.Now()}
	}
	game.Name = req.Name
	if req.Gameplay != nil {
		game.Gameplay = req.Gameplay
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
