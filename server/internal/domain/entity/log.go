// entity/log.go — Entité Log du domaine
package entity

import "time"

// LogEntry correspond à la table `logs`.
type LogEntry struct {
	ID        int                    `json:"id"`
	EventType string                 `json:"event_type"`
	Message   string                 `json:"message"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
}
