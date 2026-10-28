// index/upsert.go — Use case pour créer/mettre à jour un jeu
package index

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"ask-rules-server/internal/store"
)

// UpsertGameRequest représente la requête pour créer/mettre à jour un jeu.
type UpsertGameRequest struct {
	ID       string                 `json:"id"`
	Name     string                 `json:"name"`
	Gameplay map[string]interface{} `json:"gameplay,omitempty"`
}

// GameDetailDTO est le jeu renvoyé après création ou mise à jour.
type GameDetailDTO struct {
	ID       string                 `json:"id"`
	Name     string                 `json:"name"`
	AddedAt  time.Time              `json:"added_at"`
	Gameplay map[string]interface{} `json:"gameplay,omitempty"`
}

// Upsert crée ou met à jour un jeu.
func (s *Service) Upsert(ctx context.Context, req *UpsertGameRequest) (*GameDetailDTO, error) {
	// Validation
	if req.Name == "" {
		return nil, store.ErrInvalidGameName
	}

	id := req.ID
	if id == "" {
		id = generateGameID(req.Name)
	}

	// Mise à jour : partir du jeu existant, Save réécrit toutes les colonnes
	// (sinon fichiers, métadonnées et statistiques seraient effacés)
	game, err := s.gameRepo.FindByID(ctx, id)
	if err != nil {
		if !errors.Is(err, store.ErrGameNotFound) {
			return nil, fmt.Errorf("failed to find game: %w", err)
		}
		game = &store.Game{ID: id, AddedAt: time.Now()}
	}
	game.Name = req.Name
	if req.Gameplay != nil {
		game.Gameplay = req.Gameplay
	}

	// Sauvegarder
	if err := s.gameRepo.Save(ctx, game); err != nil {
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
