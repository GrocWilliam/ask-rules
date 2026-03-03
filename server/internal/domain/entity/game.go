// domain/entity/game.go — Entité Game (domaine métier)
package entity

import (
	"time"
)

// Game représente un jeu de société dans le domaine métier.
// Correspond à la table `games` en base de données.
type Game struct {
	ID       string                 `json:"id"`
	Name     string                 `json:"name"`      // colonne PG: jeu
	FilePath string                 `json:"file_path"` // colonne PG: fichier
	AddedAt  time.Time              `json:"added_at"`  // colonne PG: date_ajout
	Metadata map[string]interface{} `json:"metadata"`
	Stats    map[string]interface{} `json:"stats"` // colonne PG: statistiques
	Gameplay map[string]interface{} `json:"gameplay"`
}

// GameWithStats est un jeu avec des statistiques (nombre de sections).
type GameWithStats struct {
	Game
	SectionsCount int `json:"sections_count"`
}
