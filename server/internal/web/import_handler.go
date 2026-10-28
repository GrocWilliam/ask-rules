package web

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"ask-rules-server/internal/files"
	"ask-rules-server/internal/index"

	"github.com/go-chi/chi/v5"
)

const (
	maxUploadSize   = 50 << 20 // 50 MB
	maxMemoryUpload = 8 << 20  // 8 MB gardés en RAM, le reste sur disque
)

// ImportHandler gère les requêtes HTTP pour l'import de jeux.
type ImportHandler struct {
	index      *index.Service
	jobTracker *JobTracker
}

// NewImportHandler crée un nouveau handler.
func NewImportHandler(indexer *index.Service) *ImportHandler {
	return &ImportHandler{index: indexer, jobTracker: NewJobTracker()}
}

// downloadAndSaveFromURL télécharge un fichier depuis une URL et le sauvegarde dans UPLOADS_DIR/{slug}/
func downloadAndSaveFromURL(rawURL string, slug string) (string, string, error) {
	// Télécharger le fichier
	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	// Créer la requête avec un User-Agent pour éviter les blocages
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; AskRulesBot/1.0)")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("download failed with status: %d", resp.StatusCode)
	}

	log.Printf("[INFO] downloadAndSaveFromURL - Downloading from %s (Content-Type: %s, Content-Length: %s)",
		rawURL, resp.Header.Get("Content-Type"), resp.Header.Get("Content-Length"))

	// Taille plafonnée comme pour un upload
	body := bufio.NewReader(io.LimitReader(resp.Body, maxUploadSize+1))

	// L'extension vient du contenu, pas des en-têtes : un PDF servi en
	// application/octet-stream sans extension dans l'URL serait sinon indexé
	// comme du texte brut
	head, _ := body.Peek(512)
	name := extractFilenameFromURL(rawURL, resp)
	ext, err := sniffExtension(head, name)
	if err != nil {
		return "", "", err
	}

	// Nom de fichier fourni par le serveur distant : nettoyé par storage (pas de « ../ »)
	filename := strings.TrimSuffix(name, path.Ext(name)) + ext
	filePath, size, err := files.SaveFile(body, filename, slug)
	if err != nil {
		return "", "", fmt.Errorf("failed to save file: %w", err)
	}
	if size == 0 || size > maxUploadSize {
		files.DeleteFile(filePath)
		if size == 0 {
			return "", "", fmt.Errorf("downloaded file is empty (0 bytes)")
		}
		return "", "", fmt.Errorf("file too large (max 50 MB)")
	}

	log.Printf("[INFO] downloadAndSaveFromURL - Downloaded %d bytes to %s", size, filePath)
	return filePath, path.Base(filePath), nil
}

// extractFilenameFromURL extrait le nom du fichier depuis l'en-tête
// Content-Disposition ou l'URL (l'extension est ensuite fixée par sniffExtension).
func extractFilenameFromURL(rawURL string, resp *http.Response) string {
	// filename* (RFC 5987) est décodé par ParseMediaType dans params["filename"]
	if _, params, err := mime.ParseMediaType(resp.Header.Get("Content-Disposition")); err == nil && params["filename"] != "" {
		return params["filename"]
	}
	if u, err := url.Parse(rawURL); err == nil {
		if name := path.Base(u.Path); name != "/" && name != "." {
			return name
		}
	}
	return "downloaded-file"
}

// sniffExtension retourne l'extension d'après le début du contenu : .pdf, ou
// pour du texte brut celle du nom (.md) ou .txt. Les autres formats (HTML…)
// ne sont pas indexables.
func sniffExtension(head []byte, name string) (string, error) {
	contentType := http.DetectContentType(head)
	switch {
	case contentType == "application/pdf":
		return ".pdf", nil
	case strings.HasPrefix(contentType, "text/plain"):
		if strings.EqualFold(path.Ext(name), ".md") {
			return ".md", nil
		}
		return ".txt", nil
	default:
		return "", fmt.Errorf("format non supporté (%s) : PDF, TXT ou Markdown attendu", contentType)
	}
}

// importStream envoie les Server-Sent Events des imports (type dans data). Le
// traitement continue si le client se déconnecte : les envois sont alors ignorés.
type importStream struct {
	w       http.ResponseWriter
	flusher http.Flusher
	mu      sync.Mutex
	closed  bool
	stop    context.CancelFunc
}

// startImportStream ouvre le flux et envoie un ping toutes les 10 secondes pour éviter
// les timeouts proxy/navigateur. Lire le corps de la requête avant : une fois
// la réponse commencée, Go ne permet plus de le lire (upload lent).
func startImportStream(w http.ResponseWriter, r *http.Request) (*importStream, bool) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return nil, false
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ctx, stop := context.WithCancel(r.Context())
	s := &importStream{w: w, flusher: flusher, stop: stop}
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for count := 1; ; count++ {
			select {
			case <-ctx.Done():
				s.close() // fin du handler ou client déconnecté
				return
			case <-ticker.C:
				s.send("ping", map[string]interface{}{"timestamp": time.Now().Unix(), "count": count})
			}
		}
	}()
	return s, true
}

func (s *importStream) send(eventType string, data map[string]interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	merged := map[string]interface{}{"type": eventType}
	for k, v := range data {
		merged[k] = v
	}
	b, _ := json.Marshal(merged)
	if _, err := fmt.Fprintf(s.w, "data: %s\n\n", b); err != nil {
		s.closed = true // erreur d'écriture = client déconnecté
		return
	}
	s.flusher.Flush()
}

func (s *importStream) sendError(msg string) {
	s.send("error", map[string]interface{}{"error": msg})
}

// close arrête les pings ; à appeler avant la fin du handler (plus d'écriture ensuite).
func (s *importStream) close() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.stop()
}

// Import traite les requêtes POST /api/import avec Server-Sent Events.
func (h *ImportHandler) Import(w http.ResponseWriter, r *http.Request) {
	// Au-delà de maxMemoryUpload, les fichiers sont écrits dans des fichiers temporaires
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	parseErr := r.ParseMultipartForm(maxMemoryUpload)

	stream, ok := startImportStream(w, r)
	if !ok {
		return
	}
	defer stream.close()

	if parseErr != nil {
		log.Printf("[ERROR] /api/import - File too large or parse error: %v", parseErr)
		stream.sendError("File too large (max 50 MB)")
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
		stream.sendError("Game name is required")
		return
	}

	// Détecter le mode d'import (file ou url)
	importMode := r.FormValue("importMode")
	var files []*multipart.FileHeader
	var preCopiedPaths []string

	if importMode == "url" {
		// Import via URL
		urlStr := r.FormValue("url")
		if urlStr == "" {
			log.Printf("[ERROR] /api/import - Missing URL for game '%s'", gameName)
			stream.sendError("URL is required for URL import mode")
			return
		}

		log.Printf("[INFO] /api/import - Downloading file from URL: %s", urlStr)
		stream.send("downloading", map[string]interface{}{"url": urlStr})

		// Télécharger et sauvegarder directement dans le dossier du jeu
		filePath, filename, err := downloadAndSaveFromURL(urlStr, index.Slugify(gameName))
		if err != nil {
			log.Printf("[ERROR] /api/import - Failed to download from URL '%s': %v", urlStr, err)
			stream.sendError(fmt.Sprintf("Failed to download file: %v", err))
			return
		}
		preCopiedPaths = append(preCopiedPaths, filePath)

		log.Printf("[INFO] /api/import - Successfully downloaded '%s' from URL", filename)
	} else {
		// Import via fichier (mode classique)
		files = r.MultipartForm.File["fichier"]
		if len(files) == 0 {
			files = r.MultipartForm.File["files"]
		}
		if len(files) == 0 {
			files = r.MultipartForm.File["file"]
		}
		if len(files) == 0 {
			log.Printf("[ERROR] /api/import - No files provided for game '%s'", gameName)
			stream.sendError("No files provided")
			return
		}
	}

	// Mode d'import
	mode := r.FormValue("mode")
	if mode == "" {
		mode = "replace"
	}

	req := &index.ImportRequest{
		GameName:       gameName,
		Files:          files,
		PreCopiedPaths: preCopiedPaths,
		Mode:           mode,
		OnEvent:        stream.send,
	}

	// Contexte indépendant : le traitement continue même si le client se déconnecte
	log.Printf("[INFO] /api/import - Starting import for game '%s' (continues on disconnect)", gameName)
	if err := h.index.Import(context.Background(), req); err != nil {
		log.Printf("[ERROR] /api/import - Failed to import game '%s': %v", gameName, err)
		stream.sendError(err.Error())
		return
	}

	log.Printf("[INFO] /api/import - Import completed for game '%s'", gameName)
}

// startJob crée un job tracké (reconnexion possible via /api/admin/jobs/{id})
// et retourne la fonction d'envoi qui enregistre aussi les événements du job.
func (h *ImportHandler) startJob(stream *importStream, jobType JobType, gameName, gameID string) (string, func(string, map[string]interface{})) {
	jobID := h.jobTracker.CreateJob(jobType, gameName, gameID)
	stream.send("job_started", map[string]interface{}{
		"job_id":  jobID,
		"message": "Job créé, vous pouvez vous reconnecter à /api/admin/jobs/" + jobID,
	})
	return jobID, h.jobTracker.CreateEventWrapper(jobID, stream.send)
}

// Reprocess traite les requêtes POST /api/admin/games/{id}/reprocess avec Server-Sent Events.
func (h *ImportHandler) Reprocess(w http.ResponseWriter, r *http.Request) {
	stream, ok := startImportStream(w, r)
	if !ok {
		return
	}
	defer stream.close()

	gameID := chi.URLParam(r, "id")
	if gameID == "" {
		stream.sendError("Missing game ID")
		return
	}

	jobID, trackedSend := h.startJob(stream, JobTypeReprocess, "", gameID)
	req := &index.ReprocessRequest{
		GameID:  gameID,
		OnEvent: trackedSend,
	}

	// Contexte indépendant : le traitement continue même si le client se déconnecte
	log.Printf("[INFO] /api/admin/games/%s/reprocess - Starting job %s (continues on disconnect)", gameID, jobID)
	if err := h.index.Reprocess(context.Background(), req); err != nil {
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
	// game_ids optionnels (tableau vide ou absent = tous les jeux) ; erreurs de
	// décodage ignorées (body vide ou Content-Type absent = tous les jeux)
	var body struct {
		GameIDs []string `json:"game_ids"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	stream, ok := startImportStream(w, r)
	if !ok {
		return
	}
	defer stream.close()

	log.Printf("[INFO] /api/admin/reprocess-all - Starting reprocess of all games")
	if len(body.GameIDs) > 0 {
		log.Printf("[INFO] /api/admin/reprocess-all - Filtering to %d game(s): %v", len(body.GameIDs), body.GameIDs)
	}

	jobID, trackedSend := h.startJob(stream, JobTypeReprocessAll, "all", "")

	// Contexte indépendant : le traitement continue même si le client se déconnecte
	h.index.ReprocessAll(context.Background(), body.GameIDs, trackedSend)
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

	// Compteur total plutôt que len(Events) : Events est plafonné (100 derniers),
	// sa longueur cesse d'augmenter alors que des événements arrivent encore
	lastEventCount := job.EventCount

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			currentJob, exists := h.jobTracker.GetJob(jobID)
			if !exists {
				return
			}

			// Envoyer les nouveaux événements (les plus anciens ont pu être retirés)
			if newCount := currentJob.EventCount - lastEventCount; newCount > 0 {
				for _, event := range currentJob.Events[max(0, len(currentJob.Events)-newCount):] {
					data := map[string]interface{}{"type": event.Type}
					for k, v := range event.Data {
						data[k] = v
					}
					b, _ := json.Marshal(data)
					fmt.Fprintf(w, "data: %s\n\n", b)
					flusher.Flush()
				}
				lastEventCount = currentJob.EventCount
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
