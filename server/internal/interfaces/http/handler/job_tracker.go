package handler

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// JobStatus représente l'état d'un job en cours ou terminé.
type JobStatus string

const (
	JobStatusRunning   JobStatus = "running"
	JobStatusCompleted JobStatus = "completed"
	JobStatusFailed    JobStatus = "failed"
)

// JobType représente le type de job.
type JobType string

const (
	JobTypeImport       JobType = "import"
	JobTypeReprocess    JobType = "reprocess"
	JobTypeReprocessAll JobType = "reprocess_all"
)

// JobEvent représente un événement SSE capturé.
type JobEvent struct {
	Type      string                 `json:"type"`
	Data      map[string]interface{} `json:"data"`
	Timestamp time.Time              `json:"timestamp"`
}

// Job représente un job tracké avec son historique d'événements.
type Job struct {
	ID          string     `json:"id"`
	Type        JobType    `json:"type"`
	Status      JobStatus  `json:"status"`
	GameName    string     `json:"game_name,omitempty"`
	GameID      string     `json:"game_id,omitempty"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Events      []JobEvent `json:"events"`
	Error       string     `json:"error,omitempty"`

	// Compteurs pour affichage rapide
	Progress *JobProgress `json:"progress,omitempty"`

	mu sync.RWMutex
}

// JobProgress représente la progression d'un job.
type JobProgress struct {
	Current int    `json:"current"`
	Total   int    `json:"total"`
	Message string `json:"message,omitempty"`
}

// JobTracker gère les jobs en cours et leur historique.
type JobTracker struct {
	jobs map[string]*Job
	mu   sync.RWMutex

	// Configuration de rétention
	maxEvents     int           // Nombre max d'événements par job
	retentionTime time.Duration // Durée de conservation des jobs terminés
}

// NewJobTracker crée un nouveau tracker de jobs.
func NewJobTracker() *JobTracker {
	tracker := &JobTracker{
		jobs:          make(map[string]*Job),
		maxEvents:     100,            // Garder les 100 derniers événements
		retentionTime: 24 * time.Hour, // Garder les jobs terminés 24h
	}

	// Goroutine de nettoyage des vieux jobs
	go tracker.cleanupLoop()

	return tracker
}

// CreateJob crée un nouveau job et retourne son ID.
func (t *JobTracker) CreateJob(jobType JobType, gameName, gameID string) string {
	t.mu.Lock()
	defer t.mu.Unlock()

	jobID := fmt.Sprintf("%s-%d", jobType, time.Now().UnixNano())

	job := &Job{
		ID:        jobID,
		Type:      jobType,
		Status:    JobStatusRunning,
		GameName:  gameName,
		GameID:    gameID,
		StartedAt: time.Now(),
		Events:    make([]JobEvent, 0, t.maxEvents),
	}

	t.jobs[jobID] = job
	return jobID
}

// AddEvent ajoute un événement à un job.
func (t *JobTracker) AddEvent(jobID, eventType string, data map[string]interface{}) {
	t.mu.RLock()
	job, exists := t.jobs[jobID]
	t.mu.RUnlock()

	if !exists {
		return
	}

	job.mu.Lock()
	defer job.mu.Unlock()

	event := JobEvent{
		Type:      eventType,
		Data:      data,
		Timestamp: time.Now(),
	}

	// Limiter le nombre d'événements stockés
	if len(job.Events) >= t.maxEvents {
		// Garder seulement les (maxEvents - 1) derniers + le nouveau
		job.Events = append(job.Events[1:], event)
	} else {
		job.Events = append(job.Events, event)
	}

	// Mettre à jour la progression si c'est un événement de progression
	if eventType == "progress" || eventType == "embedding_progress" {
		if current, ok := data["current"].(int); ok {
			if total, ok := data["total"].(int); ok {
				if job.Progress == nil {
					job.Progress = &JobProgress{}
				}
				job.Progress.Current = current
				job.Progress.Total = total
			}
		}
		if done, ok := data["done"].(int); ok {
			if total, ok := data["total"].(int); ok {
				if job.Progress == nil {
					job.Progress = &JobProgress{}
				}
				job.Progress.Current = done
				job.Progress.Total = total
			}
		}
	}

	// Mettre à jour le message si présent
	if msg, ok := data["message"].(string); ok {
		if job.Progress == nil {
			job.Progress = &JobProgress{}
		}
		job.Progress.Message = msg
	}
}

// CompleteJob marque un job comme terminé.
func (t *JobTracker) CompleteJob(jobID string, success bool, errorMsg string) {
	t.mu.RLock()
	job, exists := t.jobs[jobID]
	t.mu.RUnlock()

	if !exists {
		return
	}

	job.mu.Lock()
	defer job.mu.Unlock()

	now := time.Now()
	job.CompletedAt = &now

	if success {
		job.Status = JobStatusCompleted
	} else {
		job.Status = JobStatusFailed
		job.Error = errorMsg
	}
}

// GetJob retourne un job par son ID.
func (t *JobTracker) GetJob(jobID string) (*Job, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	job, exists := t.jobs[jobID]
	if !exists {
		return nil, false
	}

	// Créer une copie pour éviter les race conditions
	job.mu.RLock()
	defer job.mu.RUnlock()

	jobCopy := &Job{
		ID:          job.ID,
		Type:        job.Type,
		Status:      job.Status,
		GameName:    job.GameName,
		GameID:      job.GameID,
		StartedAt:   job.StartedAt,
		CompletedAt: job.CompletedAt,
		Error:       job.Error,
		Events:      make([]JobEvent, len(job.Events)),
	}
	copy(jobCopy.Events, job.Events)

	if job.Progress != nil {
		jobCopy.Progress = &JobProgress{
			Current: job.Progress.Current,
			Total:   job.Progress.Total,
			Message: job.Progress.Message,
		}
	}

	return jobCopy, true
}

// ListJobs retourne tous les jobs (pour debug/admin).
func (t *JobTracker) ListJobs() []*Job {
	t.mu.RLock()
	defer t.mu.RUnlock()

	jobs := make([]*Job, 0, len(t.jobs))
	for _, job := range t.jobs {
		job.mu.RLock()
		jobCopy := &Job{
			ID:          job.ID,
			Type:        job.Type,
			Status:      job.Status,
			GameName:    job.GameName,
			GameID:      job.GameID,
			StartedAt:   job.StartedAt,
			CompletedAt: job.CompletedAt,
			Error:       job.Error,
		}
		if job.Progress != nil {
			jobCopy.Progress = &JobProgress{
				Current: job.Progress.Current,
				Total:   job.Progress.Total,
				Message: job.Progress.Message,
			}
		}
		job.mu.RUnlock()
		jobs = append(jobs, jobCopy)
	}

	return jobs
}

// cleanupLoop nettoie périodiquement les vieux jobs terminés.
func (t *JobTracker) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		t.cleanup()
	}
}

// cleanup supprime les jobs terminés plus vieux que retentionTime.
func (t *JobTracker) cleanup() {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	for id, job := range t.jobs {
		job.mu.RLock()
		shouldDelete := job.CompletedAt != nil && now.Sub(*job.CompletedAt) > t.retentionTime
		job.mu.RUnlock()

		if shouldDelete {
			delete(t.jobs, id)
		}
	}
}

// CreateEventWrapper crée une fonction wrapper qui enregistre les événements dans le tracker.
func (t *JobTracker) CreateEventWrapper(jobID string, originalSend func(string, map[string]interface{})) func(string, map[string]interface{}) {
	return func(eventType string, data map[string]interface{}) {
		// Enregistrer dans le tracker
		t.AddEvent(jobID, eventType, data)

		// Envoyer au client SSE (si connecté)
		if originalSend != nil {
			originalSend(eventType, data)
		}
	}
}

// MarshalJob convertit un job en JSON pour la réponse HTTP.
func MarshalJob(job *Job) ([]byte, error) {
	return json.Marshal(job)
}
