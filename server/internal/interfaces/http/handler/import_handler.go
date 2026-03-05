package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"ask-rules-server/internal/application/usecase"

	"github.com/go-chi/chi/v5"
)

const maxUploadSize = 50 << 20 // 50 MB

// ImportHandler gère les requêtes HTTP pour l'import de jeux.
type ImportHandler struct {
	importUseCase    *usecase.ImportGameUseCase
	reprocessUseCase *usecase.ReprocessGameUseCase
	jobTracker       *JobTracker
}

// NewImportHandler crée un nouveau handler.
func NewImportHandler(importUseCase *usecase.ImportGameUseCase, reprocessUseCase *usecase.ReprocessGameUseCase) *ImportHandler {
	return &ImportHandler{
		importUseCase:    importUseCase,
		reprocessUseCase: reprocessUseCase,
		jobTracker:       NewJobTracker(),
	}
}

// Import traite les requêtes POST /api/import avec Server-Sent Events.
func (h *ImportHandler) Import(w http.ResponseWriter, r *http.Request) {
	// Configurer SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		log.Printf("[ERROR] /api/import - SSE not supported")
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Détection de déconnexion client
	clientDisconnected := false
	var clientMu sync.RWMutex

	// Fonction d'envoi SSE (ignore les erreurs si client déconnecté)
	var sendMu sync.Mutex
	send := func(eventType string, data map[string]interface{}) {
		clientMu.RLock()
		disconnected := clientDisconnected
		clientMu.RUnlock()

		if disconnected {
			// Client déconnecté, ne pas envoyer (traitement continue en arrière-plan)
			return
		}

		sendMu.Lock()
		defer sendMu.Unlock()
		merged := map[string]interface{}{"type": eventType}
		for k, v := range data {
			merged[k] = v
		}
		b, _ := json.Marshal(merged)
		_, err := fmt.Fprintf(w, "data: %s\n\n", b)
		if err != nil {
			// Erreur d'écriture = client déconnecté
			clientMu.Lock()
			clientDisconnected = true
			clientMu.Unlock()
			return
		}
		flusher.Flush()
	}

	sendError := func(msg string) {
		send("error", map[string]interface{}{"error": msg})
	}

	// Surveillance de la déconnexion client (pour logs uniquement)
	go func() {
		<-r.Context().Done()
		clientMu.Lock()
		if !clientDisconnected {
			clientDisconnected = true
			log.Printf("[INFO] /api/import - Client déconnecté, traitement continue en arrière-plan")
		}
		clientMu.Unlock()
	}()

	// Heartbeat pour garder la connexion SSE ouverte
	// Envoie un ping toutes les 10 secondes pour éviter les timeouts proxy/navigateur
	heartbeatCtx, heartbeatCancel := context.WithCancel(context.Background())
	defer heartbeatCancel()

	pingCount := 0
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatCtx.Done():
				log.Printf("[INFO] /api/import - SSE connection closed after %d pings", pingCount)
				return
			case <-r.Context().Done():
				// Client déconnecté, arrêter le heartbeat
				return
			case <-ticker.C:
				pingCount++
				send("ping", map[string]interface{}{"timestamp": time.Now().Unix(), "count": pingCount})
				if pingCount%6 == 0 {
					log.Printf("[DEBUG] /api/import - SSE heartbeat: %d pings sent (%.1f min)", pingCount, float64(pingCount)/6)
				}
			}
		}
	}()

	// Parser le formulaire multipart
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		log.Printf("[ERROR] /api/import - File too large or parse error: %v", err)
		sendError("File too large (max 50 MB)")
		return
	}

	// Récupérer le nom du jeu
	gameName := r.FormValue("gameName")
	if gameName == "" {
		gameName = r.FormValue("game")
	}
	if gameName == "" {
		gameName = r.FormValue("jeu")
	}
	if gameName == "" {
		log.Printf("[ERROR] /api/import - Missing game name")
		sendError("Game name is required")
		return
	}

	// Récupérer les fichiers
	files := r.MultipartForm.File["fichier"]
	if len(files) == 0 {
		files = r.MultipartForm.File["files"]
	}
	if len(files) == 0 {
		files = r.MultipartForm.File["file"]
	}
	if len(files) == 0 {
		log.Printf("[ERROR] /api/import - No files provided for game '%s'", gameName)
		sendError("No files provided")
		return
	}

	// Mode d'import
	mode := r.FormValue("mode")
	if mode == "" {
		mode = "replace"
	}

	// Créer la requête
	req := &usecase.ImportRequest{
		GameName: gameName,
		Files:    files,
		Mode:     mode,
		OnEvent:  send,
	}

	// Exécuter le use case avec un contexte indépendant
	// Le traitement continue même si le client SSE se déconnecte
	processCtx := context.Background()
	log.Printf("[INFO] /api/import - Starting import for game '%s' (continues on disconnect)", gameName)

	if err := h.importUseCase.Execute(processCtx, req); err != nil {
		log.Printf("[ERROR] /api/import - Failed to import game '%s': %v", gameName, err)
		sendError(err.Error())
		return
	}

	log.Printf("[INFO] /api/import - Import completed for game '%s'", gameName)
}

// Reprocess traite les requêtes POST /api/admin/games/{id}/reprocess avec Server-Sent Events.
func (h *ImportHandler) Reprocess(w http.ResponseWriter, r *http.Request) {
	// Configurer SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Détection de déconnexion client
	clientDisconnected := false
	var clientMu sync.RWMutex

	var sendMu sync.Mutex
	send := func(eventType string, data map[string]interface{}) {
		clientMu.RLock()
		disconnected := clientDisconnected
		clientMu.RUnlock()

		if disconnected {
			return
		}

		sendMu.Lock()
		defer sendMu.Unlock()
		merged := map[string]interface{}{"type": eventType}
		for k, v := range data {
			merged[k] = v
		}
		b, _ := json.Marshal(merged)
		_, err := fmt.Fprintf(w, "data: %s\n\n", b)
		if err != nil {
			clientMu.Lock()
			clientDisconnected = true
			clientMu.Unlock()
			return
		}
		flusher.Flush()
	}

	go func() {
		<-r.Context().Done()
		clientMu.Lock()
		if !clientDisconnected {
			clientDisconnected = true
			log.Printf("[INFO] /api/admin/games/reprocess - Client déconnecté, traitement continue")
		}
		clientMu.Unlock()
	}()

	heartbeatCtx, heartbeatCancel := context.WithCancel(context.Background())
	defer heartbeatCancel()

	pingCount := 0
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatCtx.Done():
				log.Printf("[INFO] /api/admin/games/reprocess - SSE connection closed after %d pings", pingCount)
				return
			case <-r.Context().Done():
				return
			case <-ticker.C:
				pingCount++
				send("ping", map[string]interface{}{"timestamp": time.Now().Unix(), "count": pingCount})
				if pingCount%6 == 0 {
					log.Printf("[DEBUG] /api/admin/games/reprocess - SSE heartbeat: %d pings sent (%.1f min)", pingCount, float64(pingCount)/6)
				}
			}
		}
	}()

	gameID := chi.URLParam(r, "id")
	if gameID == "" {
		send("error", map[string]interface{}{"error": "Missing game ID"})
		return
	}

	// Créer un job tracké pour permettre la reconnexion
	jobID := h.jobTracker.CreateJob(JobTypeReprocess, "", gameID)

	// Envoyer le job ID immédiatement pour permettre la reconnexion
	send("job_started", map[string]interface{}{
		"job_id":  jobID,
		"message": "Job créé, vous pouvez vous reconnecter à /api/admin/jobs/" + jobID,
	})

	// Wrapper la fonction send pour enregistrer dans le tracker
	trackedSend := h.jobTracker.CreateEventWrapper(jobID, send)

	req := &usecase.ReprocessRequest{
		GameID:  gameID,
		OnEvent: trackedSend,
	}

	// Utiliser un contexte indépendant pour continuer même après déconnexion
	processCtx := context.Background()
	log.Printf("[INFO] /api/admin/games/%s/reprocess - Starting job %s (continues on disconnect)", gameID, jobID)

	if err := h.reprocessUseCase.Execute(processCtx, req); err != nil {
		log.Printf("[ERROR] /api/admin/games/%s/reprocess - Failed: %v", gameID, err)
		h.jobTracker.CompleteJob(jobID, false, err.Error())
		trackedSend("error", map[string]interface{}{"error": err.Error()})
		return
	}

	h.jobTracker.CompleteJob(jobID, true, "")
	log.Printf("[INFO] /api/admin/games/%s/reprocess - Completed job %s", gameID, jobID)
}

// ReprocessAll traite les requêtes POST /api/admin/reprocess-all avec Server-Sent Events.
func (h *ImportHandler) ReprocessAll(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Détection de déconnexion client
	clientDisconnected := false
	var clientMu sync.RWMutex

	var sendMu sync.Mutex
	send := func(eventType string, data map[string]interface{}) {
		clientMu.RLock()
		disconnected := clientDisconnected
		clientMu.RUnlock()

		if disconnected {
			return
		}

		sendMu.Lock()
		defer sendMu.Unlock()
		merged := map[string]interface{}{"type": eventType}
		for k, v := range data {
			merged[k] = v
		}
		b, _ := json.Marshal(merged)
		_, err := fmt.Fprintf(w, "data: %s\n\n", b)
		if err != nil {
			clientMu.Lock()
			clientDisconnected = true
			clientMu.Unlock()
			return
		}
		flusher.Flush()
	}

	go func() {
		<-r.Context().Done()
		clientMu.Lock()
		if !clientDisconnected {
			clientDisconnected = true
			log.Printf("[INFO] /api/admin/reprocess-all - Client déconnecté, traitement continue")
		}
		clientMu.Unlock()
	}()

	heartbeatCtx, heartbeatCancel := context.WithCancel(context.Background())
	defer heartbeatCancel()

	pingCount := 0
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatCtx.Done():
				log.Printf("[INFO] /api/admin/reprocess-all - SSE connection closed after %d pings", pingCount)
				return
			case <-r.Context().Done():
				return
			case <-ticker.C:
				pingCount++
				send("ping", map[string]interface{}{"timestamp": time.Now().Unix(), "count": pingCount})
				if pingCount%6 == 0 {
					log.Printf("[DEBUG] /api/admin/reprocess-all - SSE heartbeat: %d pings sent (%.1f min)", pingCount, float64(pingCount)/6)
				}
			}
		}
	}()

	log.Printf("[INFO] /api/admin/reprocess-all - Starting reprocess of all games")

	// Lire les game_ids optionnels depuis le body JSON (tableau vide ou absent = tous les jeux)
	var body struct {
		GameIDs []string `json:"game_ids"`
	}
	// Ignorer les erreurs de décodage (body vide ou Content-Type absent = tous les jeux)
	_ = json.NewDecoder(r.Body).Decode(&body)

	if len(body.GameIDs) > 0 {
		log.Printf("[INFO] /api/admin/reprocess-all - Filtering to %d game(s): %v", len(body.GameIDs), body.GameIDs)
	}

	// Créer un job tracké pour permettre la reconnexion
	jobID := h.jobTracker.CreateJob(JobTypeReprocessAll, "all", "")

	// Envoyer le job ID immédiatement
	send("job_started", map[string]interface{}{
		"job_id":  jobID,
		"message": "Job créé, vous pouvez vous reconnecter à /api/admin/jobs/" + jobID,
	})

	// Wrapper la fonction send pour enregistrer dans le tracker
	trackedSend := h.jobTracker.CreateEventWrapper(jobID, send)

	// Utiliser un contexte indépendant pour continuer même après déconnexion
	processCtx := context.Background()
	h.reprocessUseCase.ExecuteAll(processCtx, body.GameIDs, trackedSend)
	h.jobTracker.CompleteJob(jobID, true, "")
	log.Printf("[INFO] /api/admin/reprocess-all - Completed job %s", jobID)
}

// GetJobStatus retourne le statut d'un job pour permettre la reprise après déconnexion.
func (h *ImportHandler) GetJobStatus(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")
	if jobID == "" {
		http.Error(w, "Missing job ID", http.StatusBadRequest)
		return
	}

	job, exists := h.jobTracker.GetJob(jobID)
	if !exists {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job)
}

// StreamJobEvents permet de se reconnecter au stream SSE d'un job en cours.
func (h *ImportHandler) StreamJobEvents(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")
	if jobID == "" {
		http.Error(w, "Missing job ID", http.StatusBadRequest)
		return
	}

	job, exists := h.jobTracker.GetJob(jobID)
	if !exists {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	// Configurer SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Envoyer immédiatement l'historique des événements
	for _, event := range job.Events {
		data := map[string]interface{}{"type": event.Type}
		for k, v := range event.Data {
			data[k] = v
		}
		b, _ := json.Marshal(data)
		fmt.Fprintf(w, "data: %s\n\n", b)
		flusher.Flush()
	}

	// Si le job est terminé, envoyer un événement final et fermer
	if job.Status == JobStatusCompleted || job.Status == JobStatusFailed {
		finalEvent := map[string]interface{}{
			"type":   "job_status",
			"status": string(job.Status),
		}
		if job.Error != "" {
			finalEvent["error"] = job.Error
		}
		b, _ := json.Marshal(finalEvent)
		fmt.Fprintf(w, "data: %s\n\n", b)
		flusher.Flush()
		return
	}

	// Si le job est en cours, continuer à streamer les nouveaux événements
	// On va poller le tracker toutes les secondes pour les nouveaux événements
	ctx := r.Context()
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	lastEventIndex := len(job.Events)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			currentJob, exists := h.jobTracker.GetJob(jobID)
			if !exists {
				return
			}

			// Envoyer les nouveaux événements depuis lastEventIndex
			if len(currentJob.Events) > lastEventIndex {
				for i := lastEventIndex; i < len(currentJob.Events); i++ {
					event := currentJob.Events[i]
					data := map[string]interface{}{"type": event.Type}
					for k, v := range event.Data {
						data[k] = v
					}
					b, _ := json.Marshal(data)
					fmt.Fprintf(w, "data: %s\n\n", b)
					flusher.Flush()
				}
				lastEventIndex = len(currentJob.Events)
			}

			// Si le job est terminé, envoyer l'événement final et fermer
			if currentJob.Status == JobStatusCompleted || currentJob.Status == JobStatusFailed {
				finalEvent := map[string]interface{}{
					"type":   "job_status",
					"status": string(currentJob.Status),
				}
				if currentJob.Error != "" {
					finalEvent["error"] = currentJob.Error
				}
				b, _ := json.Marshal(finalEvent)
				fmt.Fprintf(w, "data: %s\n\n", b)
				flusher.Flush()
				return
			}
		}
	}
}

// ListJobs retourne la liste de tous les jobs (pour debug admin).
func (h *ImportHandler) ListJobs(w http.ResponseWriter, r *http.Request) {
	jobs := h.jobTracker.ListJobs()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jobs)
}
