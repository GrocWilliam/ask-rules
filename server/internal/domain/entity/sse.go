// entity/sse.go — Événements Server-Sent Events
package entity

// SSEEvent est un événement SSE pour le streaming d'import.
type SSEEvent struct {
	Type    string                 `json:"type"`
	Data    map[string]interface{} `json:"data,omitempty"`
	Message string                 `json:"message,omitempty"`
}
