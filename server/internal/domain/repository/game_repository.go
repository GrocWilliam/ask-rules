// domain/repository/game_repository.go — Interface repository pour les jeux
package repository

import (
	"context"

	"ask-rules-server/internal/domain/entity"
)

// GameRepository définit les opérations sur les jeux.
// Cette interface est indépendante de l'implémentation (PostgreSQL, MongoDB, etc.)
type GameRepository interface {
	// FindByID retourne un jeu par son ID.
	// Retourne ErrGameNotFound si le jeu n'existe pas.
	FindByID(ctx context.Context, id string) (*entity.Game, error)

	// FindByName retourne un jeu par son nom (recherche insensible à la casse).
	// Retourne ErrGameNotFound si aucun jeu ne correspond.
	FindByName(ctx context.Context, name string) (*entity.Game, error)

	// List retourne tous les jeux avec leur compte de sections.
	List(ctx context.Context) ([]*entity.GameWithStats, error)

	// Save crée ou met à jour un jeu.
	// Si l'ID existe déjà, met à jour, sinon crée.
	Save(ctx context.Context, game *entity.Game) error

	// Delete supprime un jeu et toutes ses sections.
	Delete(ctx context.Context, id string) error

	// UpdateGameplay met à jour uniquement le champ gameplay d'un jeu.
	UpdateGameplay(ctx context.Context, id string, gameplay map[string]interface{}) error
}
