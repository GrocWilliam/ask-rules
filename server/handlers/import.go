// handlers/import.go — Import de fichiers avec progression SSE
package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"ask-rules-server/config"
	"ask-rules-server/db"
	"ask-rules-server/logger"
	"ask-rules-server/models"
	"ask-rules-server/pipeline"
	"ask-rules-server/storage"
)

const maxUploadSize = 50 << 20 // 50 MB

// ImportSSE gère l'upload + import d'un fichier pour un jeu (réponse SSE).
func ImportSSE(w http.ResponseWriter, r *http.Request) {
	// Configurer SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE non supporté", http.StatusInternalServerError)
		return
	}

	send := func(eventType string, data map[string]interface{}) {
		merged := map[string]interface{}{"type": eventType}
		for k, v := range data {
			merged[k] = v
		}
		b, _ := json.Marshal(merged)
		fmt.Fprintf(w, "data: %s\n\n", b)
		flusher.Flush()
	}

	sendError := func(msg string) {
		send("error", map[string]interface{}{"error": msg})
	}

	// Parser le formulaire multipart
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		sendError("Fichier trop volumineux (max 50 MB)")
		return
	}

	gameName := r.FormValue("gameName")
	if gameName == "" {
		gameName = r.FormValue("jeu")
	}
	if gameName == "" {
		gameName = r.FormValue("game")
	}
	if gameName == "" {
		sendError("Nom du jeu manquant")
		return
	}

	files := r.MultipartForm.File["fichier"]
	if len(files) == 0 {
		files = r.MultipartForm.File["files"]
	}
	if len(files) == 0 {
		if f := r.MultipartForm.File["file"]; len(f) > 0 {
			files = f
		}
	}
	if len(files) == 0 {
		sendError("Aucun fichier fourni")
		return
	}

	send("start", map[string]interface{}{"jeu": gameName, "files": len(files)})

	ctx := r.Context()

	// Heartbeat : envoyer un ping toutes les 15 s pour éviter les timeouts proxy/navigateur
	doneCh := make(chan struct{})
	defer close(doneCh)
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				fmt.Fprintf(w, ": ping\n\n")
				flusher.Flush()
			case <-doneCh:
				return
			}
		}
	}()

	// Mode d'import : "replace" (défaut) ou "merge"
	mode := r.FormValue("mode")
	if mode == "" {
		mode = "replace"
	}

	// Créer ou trouver le jeu
	existingGame, _ := db.FindGameByName(ctx, gameName)
	isNew := existingGame == nil
	game := existingGame
	if isNew {
		game = &models.Game{
			ID:       newImportUUID(),
			Name:     gameName,
			Metadata: map[string]interface{}{},
			Stats:    map[string]interface{}{},
			Gameplay: map[string]interface{}{},
		}
	}

	// Sauvegarder les fichiers
	var filePaths []string
	for _, fh := range files {
		send("uploading", map[string]interface{}{"file": fh.Filename})
		fp, err := saveFile(fh, slugify(gameName))
		if err != nil {
			sendError(fmt.Sprintf("Erreur upload %s: %s", fh.Filename, err))
			continue
		}
		filePaths = append(filePaths, fp)
	}

	if len(filePaths) == 0 {
		sendError("Aucun fichier sauvegardé")
		return
	}

	// Mettre à jour la liste des fichiers du jeu
	game.FilePath = filePaths[0] // champ principal
	game.Stats["files"] = filePaths
	if err := db.UpsertGame(ctx, game); err != nil {
		sendError("Erreur base de données: " + err.Error())
		return
	}

	// Si mode "replace" et que le jeu existait déjà, supprimer ses sections
	if !isNew && mode == "replace" {
		if err := db.DeleteSections(ctx, game.ID); err != nil {
			sendError("Erreur suppression des sections existantes: " + err.Error())
			return
		}
		send("replacing", map[string]interface{}{"jeu": gameName})
	}

	// Lancer le pipeline
	start := time.Now()
	importErr := pipeline.Run(ctx, pipeline.ImportOptions{
		GameName:  gameName,
		FilePaths: filePaths,
		OnEvent: func(event string, data map[string]interface{}) {
			send(event, data)
		},
	})

	if importErr != nil {
		logger.ImportError(ctx, gameName, importErr.Error())
		sendError("Erreur import: " + importErr.Error())
		return
	}

	logger.GameAdded(ctx, gameName, len(filePaths))
	// Libérer mémoire après import
	runtime.GC()
	send("complete", map[string]interface{}{
		"jeu":      gameName,
		"files":    len(filePaths),
		"duration": time.Since(start).Milliseconds(),
	})
}

// ReprocessGame réindexe un jeu existant.
func ReprocessGame(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, _ := w.(http.Flusher)

	send := func(t string, d map[string]interface{}) {
		merged := map[string]interface{}{"type": t}
		for k, v := range d {
			merged[k] = v
		}
		b, _ := json.Marshal(merged)
		fmt.Fprintf(w, "data: %s\n\n", b)
		if flusher != nil {
			flusher.Flush()
		}
	}

	// Heartbeat ping toutes les 15 s
	doneCh := make(chan struct{})
	defer close(doneCh)
	if flusher != nil {
		go func() {
			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					fmt.Fprintf(w, ": ping\n\n")
					flusher.Flush()
				case <-doneCh:
					return
				}
			}
		}()
	}

	var body struct {
		ID string `json:"id"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	game, err := db.FindGame(r.Context(), body.ID)
	if err != nil || game == nil {
		send("error", map[string]interface{}{"error": "Jeu introuvable"})
		return
	}

	// Récupérer les fichiers depuis les stats
	var filePaths []string
	if fps, ok := game.Stats["files"].([]interface{}); ok {
		for _, fp := range fps {
			if s, ok := fp.(string); ok {
				filePaths = append(filePaths, s)
			}
		}
	}

	if len(filePaths) == 0 && game.FilePath != "" {
		filePaths = strings.Split(game.FilePath, "+")
		for i := range filePaths {
			filePaths[i] = strings.TrimSpace(filePaths[i])
		}
	}

	// Vérifier que les fichiers sont accessibles avant de lancer le pipeline
	var missingFiles []string
	for _, fp := range filePaths {
		absPath := filepath.Join(config.C.UploadsDir, fp)
		if _, err := os.Stat(absPath); os.IsNotExist(err) {
			missingFiles = append(missingFiles, absPath)
		}
	}

	if len(missingFiles) > 0 {
		send("error", map[string]interface{}{
			"error": fmt.Sprintf("Fichier(s) introuvable(s) dans %s : %v",
				config.C.UploadsDir, missingFiles),
		})
		return
	}

	if err := db.DeleteSections(r.Context(), game.ID); err != nil {
		send("error", map[string]interface{}{"error": "Erreur suppression sections"})
		return
	}

	pipeline.Run(r.Context(), pipeline.ImportOptions{
		GameName:  game.Name,
		FilePaths: filePaths,
		OnEvent:   func(t string, d map[string]interface{}) { send(t, d) },
	})

	logger.GameReprocessed(r.Context(), game.Name)
	send("complete", map[string]interface{}{"jeu": game.Name})
}

// ReprocessAll réindexe tous les jeux existants avec progression SSE.
// Route : POST /api/admin/reprocess-all
func ReprocessAll(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, _ := w.(http.Flusher)

	send := func(t string, d map[string]interface{}) {
		merged := map[string]interface{}{"type": t}
		for k, v := range d {
			merged[k] = v
		}
		b, _ := json.Marshal(merged)
		fmt.Fprintf(w, "data: %s\n\n", b)
		if flusher != nil {
			flusher.Flush()
		}
	}

	// Heartbeat ping toutes les 15 s
	doneCh2 := make(chan struct{})
	defer close(doneCh2)

	if flusher != nil {
		go func() {
			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					fmt.Fprintf(w, ": ping\n\n")
					flusher.Flush()
				case <-doneCh2:
					return
				}
			}
		}()
	}

	ctx := r.Context()

	games, err := db.ListGames(ctx)
	if err != nil {
		send("error", map[string]interface{}{"error": "Impossible de charger les jeux: " + err.Error()})
		return
	}
	if len(games) == 0 {
		send("complete", map[string]interface{}{"total": 0, "success": 0, "errors": 0})
		return
	}

	send("start", map[string]interface{}{"total": len(games)})

	globalStart := time.Now()
	successCount := 0
	errorCount := 0

	for i, gws := range games {
		// Vérifier si le client a abandonné
		select {
		case <-ctx.Done():
			return
		default:
		}

		game := &gws.Game
		send("game_start", map[string]interface{}{
			"game":  game.Name,
			"index": i + 1,
			"total": len(games),
		})

		// Récupérer les fichiers du jeu
		var filePaths []string
		if fps, ok := game.Stats["files"].([]interface{}); ok {
			for _, fp := range fps {
				if s, ok := fp.(string); ok {
					filePaths = append(filePaths, s)
				}
			}
		}
		if len(filePaths) == 0 && game.FilePath != "" {
			filePaths = strings.Split(game.FilePath, "+")
			for i := range filePaths {
				filePaths[i] = strings.TrimSpace(filePaths[i])
			}
		}

		if len(filePaths) == 0 {
			send("game_error", map[string]interface{}{
				"game":  game.Name,
				"index": i + 1,
				"error": "Aucun fichier associé",
			})
			errorCount++
			continue
		}

		// Vérifier que les fichiers sont accessibles avant de lancer le pipeline
		var missing []string
		for _, fp := range filePaths {
			absPath := filepath.Join(config.C.UploadsDir, fp)
			if _, err := os.Stat(absPath); os.IsNotExist(err) {
				missing = append(missing, absPath)
			}
		}
		if len(missing) > 0 {
			send("game_error", map[string]interface{}{
				"game":  game.Name,
				"index": i + 1,
				"error": fmt.Sprintf("Fichier(s) introuvable(s) dans %s : %v",
					config.C.UploadsDir, missing),
			})
			errorCount++
			continue
		}

		if err := db.DeleteSections(ctx, game.ID); err != nil {
			send("game_error", map[string]interface{}{
				"game":  game.Name,
				"index": i + 1,
				"error": "Erreur suppression sections: " + err.Error(),
			})
			errorCount++
			continue
		}

		runErr := pipeline.Run(ctx, pipeline.ImportOptions{
			GameName:  game.Name,
			FilePaths: filePaths,
			OnEvent: func(t string, d map[string]interface{}) {
				// Préfixer les events pipeline avec le contexte du jeu en cours
				d["game"] = game.Name
				d["index"] = i + 1
				send(t, d)
			},
		})

		if runErr != nil {
			send("game_error", map[string]interface{}{
				"game":  game.Name,
				"index": i + 1,
				"error": runErr.Error(),
			})
			errorCount++
			continue
		}

		logger.GameReprocessed(ctx, game.Name)
		send("game_done", map[string]interface{}{
			"game":  game.Name,
			"index": i + 1,
			"total": len(games),
		})
		successCount++

		// Libérer mémoire explicitement après chaque jeu traité
		// pour éviter l'accumulation de RAM lors du retraitement de nombreux jeux
		runtime.GC()
	}

	send("complete", map[string]interface{}{
		"total":    len(games),
		"success":  successCount,
		"errors":   errorCount,
		"duration": time.Since(globalStart).Milliseconds(),
	})
}

func saveFile(fh *multipart.FileHeader, slug string) (string, error) {
	return storage.SaveUploadedFile(fh, slug)
}

func slugify(name string) string {
	result := make([]byte, 0, len(name))
	for _, c := range name {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			result = append(result, byte(c))
		case c >= 'A' && c <= 'Z':
			result = append(result, byte(c+32))
		case c == ' ', c == '-', c == '_':
			result = append(result, '-')
		}
	}
	return string(result)
}

func newImportUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b[:4]) + "-" + hex.EncodeToString(b[4:6]) + "-" +
		hex.EncodeToString(b[6:8]) + "-" + hex.EncodeToString(b[8:10]) + "-" +
		hex.EncodeToString(b[10:])
}
