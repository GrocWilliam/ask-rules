// infrastructure/service/pipeline_adapter_integrated.go — Pipeline d'indexation intégré
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"ask-rules-server/internal/application/usecase"
	"ask-rules-server/internal/domain/entity"
	"ask-rules-server/internal/domain/repository"
	"ask-rules-server/internal/domain/service"
	"ask-rules-server/internal/domain/service/nlp"
	"ask-rules-server/internal/infrastructure/storage"
)

// ──────────────────────────────────────────────────────────────────────────────
// ADAPTER
// ──────────────────────────────────────────────────────────────────────────────

// PipelineAdapter adapte le pipeline d'indexation à l'interface domaine.
type PipelineAdapter struct {
	embedder    service.EmbedderService
	gameRepo    repository.GameRepository
	sectionRepo repository.SectionRepository
}

// NewPipeline crée un nouvel adapter pour le pipeline.
func NewPipeline(gameRepo repository.GameRepository, sectionRepo repository.SectionRepository, embedder service.EmbedderService) usecase.PipelineService {
	return &PipelineAdapter{
		embedder:    embedder,
		gameRepo:    gameRepo,
		sectionRepo: sectionRepo,
	}
}

// Process exécute le pipeline d'import.
func (p *PipelineAdapter) Process(ctx context.Context, opts *usecase.ImportOptions) error {
	err := p.run(ctx, opts.GameName, opts.FilePaths, opts.OnEvent)
	// Libérer le modèle ONNX de la RAM après chaque traitement batch
	if r, ok := p.embedder.(service.ReleasableEmbedder); ok {
		r.Release()
	}
	return err
}

// ──────────────────────────────────────────────────────────────────────────────
// PIPELINE (orchestration)
// ──────────────────────────────────────────────────────────────────────────────

// run exécute le pipeline complet pour un jeu.
func (p *PipelineAdapter) run(ctx context.Context, gameName string, filePaths []string, onEvent func(string, map[string]interface{})) error {
	emit := onEvent
	if emit == nil {
		emit = func(string, map[string]interface{}) {}
	}

	emit("start", map[string]interface{}{"game": gameName, "files": len(filePaths)})

	// Chercher ou créer l'entrée en base
	game, err := p.gameRepo.FindByName(ctx, gameName)
	if err != nil || game == nil {
		return fmt.Errorf("jeu introuvable en base : %s", gameName)
	}

	totalChunks := 0
	const maxChunksForGameplay = 150
	var gameplayChunks []string
	var metaChunks []string

	for _, fp := range filePaths {
		chunkCount := p.processFileStreaming(ctx, game, fp, emit, &totalChunks, &gameplayChunks, &metaChunks, maxChunksForGameplay)
		if chunkCount < 0 {
			emit("file_error", map[string]interface{}{"file": fp, "error": "Erreur traitement fichier"})
			continue
		}
	}

	// ── Extraction des métadonnées du jeu ─────────────────────────────────────
	if len(metaChunks) > 0 {
		gameMeta := nlp.ExtractGameMeta(metaChunks)
		if len(gameMeta) > 0 {
			if game.Metadata == nil {
				game.Metadata = map[string]interface{}{}
			}
			for k, v := range gameMeta {
				game.Metadata[k] = v
			}
			_ = p.gameRepo.Save(ctx, game)
		}
	}
	metaChunks = nil

	// ── Extraction du gameplay ─────────────────────────────────────────────────
	if len(gameplayChunks) > 0 {
		emit("gameplay", map[string]interface{}{"status": "extracting", "chunks": len(gameplayChunks)})
		gameplayData := nlp.ExtractGameplay(gameplayChunks)
		gameplayChunks = nil

		if !gameplayData.IsEmpty() {
			if gMap := gameplayDataToMap(gameplayData); gMap != nil {
				if saveErr := p.gameRepo.UpdateGameplay(ctx, game.ID, gMap); saveErr != nil {
					emit("gameplay_error", map[string]interface{}{"error": saveErr.Error()})
				} else {
					emit("gameplay", map[string]interface{}{
						"status":    "done",
						"mechanics": len(gameplayData.Mechanics),
						"phases":    len(gameplayData.Phases),
						"has_setup": gameplayData.Setup != nil,
						"has_turns": gameplayData.Turns != nil,
						"has_end":   gameplayData.EndGame != nil,
					})
				}
			}
		}
	}

	emit("done", map[string]interface{}{
		"game":         gameName,
		"total_chunks": totalChunks,
	})
	return nil
}

// processFileStreaming traite un fichier en streaming.
func (p *PipelineAdapter) processFileStreaming(
	ctx context.Context,
	game *entity.Game,
	fp string,
	emit func(string, map[string]interface{}),
	total *int,
	gameplayChunks *[]string,
	metaChunks *[]string,
	maxChunks int,
) int {
	absPath := storage.GetAbsolutePath(fp)

	emit("extracting", map[string]interface{}{"file": fp})
	pages, err := extractText(absPath)
	if err != nil {
		return -1
	}
	textLength := 0
	for _, pg := range pages {
		textLength += len(pg.Text)
	}
	if textLength == 0 {
		return -1
	}

	emit("chunking", map[string]interface{}{"file": fp, "text_length": textLength})
	chunks := chunkPages(pages)
	pages = nil

	emit("embedding", map[string]interface{}{"file": fp, "chunks": len(chunks)})

	processedCount := 0
	for i, chunk := range chunks {
		select {
		case <-ctx.Done():
			return processedCount
		default:
		}

		cleanText := sanitizeUTF8(chunk.Text)

		if isLowQualityText(cleanText) {
			continue
		}

		vec, embErr := p.embedder.EmbedPassage(ctx, cleanText)
		if embErr != nil {
			emit("section_error", map[string]interface{}{"index": i, "error": "embedding: " + embErr.Error()})
			vec = nil
		}

		// NLP
		titleCandidate := extractTitleCandidate(cleanText)
		sectionType := nlp.DetectSectionType(titleCandidate, cleanText)
		mechanics := nlp.DetectMechanics(cleanText)

		section := entity.Section{
			ID:            newUUID(),
			GameID:        game.ID,
			Title:         generateTitle(sectionType, cleanText, i),
			SectionType:   sectionType,
			Text:          cleanText,
			Summary:       extractFirstSentence(cleanText, 220),
			Mechanics:     mechanics,
			Embedding:     vecToFloat64(vec),
			PageStart:     intPtr(chunk.PageStart),
			PageEnd:       intPtr(chunk.PageEnd),
			SourceFile:    fp,
			HierarchyPath: sectionType,
			ChunkIndex:    i,
			TotalChunks:   len(chunks),
		}

		if err := p.sectionRepo.Insert(ctx, &section); err != nil {
			emit("section_error", map[string]interface{}{"index": i, "error": err.Error()})
		} else {
			*total++
			processedCount++

			if len(*metaChunks) < 5 {
				*metaChunks = append(*metaChunks, cleanText)
			}
			if len(*gameplayChunks) < maxChunks {
				*gameplayChunks = append(*gameplayChunks, cleanText)
			}
		}

		// Envoyer un événement de progression tous les 3 chunks (au lieu de 10)
		// pour éviter les timeouts durant les embeddings longs
		if i%3 == 0 || i == len(chunks)-1 {
			emit("progress", map[string]interface{}{"file": fp, "done": i + 1, "total": len(chunks)})
		}
	}

	return processedCount
}

// gameplayDataToMap convertit GameplayData en map via JSON.
func gameplayDataToMap(gd *nlp.GameplayData) map[string]interface{} {
	b, err := json.Marshal(gd)
	if err != nil {
		return nil
	}
	var m map[string]interface{}
	_ = json.Unmarshal(b, &m)
	return m
}

func vecToFloat64(v []float32) []float64 {
	if v == nil {
		return nil
	}
	out := make([]float64, len(v))
	for i, f := range v {
		out[i] = float64(f)
	}
	return out
}

func intPtr(v int) *int { return &v }

func newUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func sanitizeUTF8(s string) string {
	return strings.ToValidUTF8(s, "")
}

// extractTitleCandidate extrait la première ligne significative d'un texte pour servir de titre candidat.
func extractTitleCandidate(text string) string {
	for _, line := range strings.SplitN(text, "\n", 10) {
		line = strings.TrimSpace(line)
		runes := []rune(line)
		if len(runes) >= 4 && len(runes) <= 80 {
			allDigitsOrPunct := true
			letterCount := 0
			for _, r := range runes {
				if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r > 127 {
					allDigitsOrPunct = false
					letterCount++
				}
			}
			if !allDigitsOrPunct && letterCount >= 3 {
				return line
			}
		}
	}
	return ""
}

func generateTitle(sectionType, text string, index int) string {
	sectionLabels := map[string]string{
		"setup":     "Mise en place",
		"turn":      "Tour de jeu",
		"scoring":   "Score & Victoire",
		"end":       "Fin de partie",
		"component": "Composants",
		"special":   "Règle spéciale",
		"example":   "Exemple",
	}

	firstLine := ""
	for _, line := range strings.SplitN(text, "\n", 10) {
		line = strings.TrimSpace(line)
		runes := []rune(line)
		if len(runes) >= 4 && len(runes) <= 80 {
			allDigitsOrPunct := true
			letterCount := 0
			for _, r := range runes {
				if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r > 127 {
					allDigitsOrPunct = false
					letterCount++
				}
			}
			if !allDigitsOrPunct && letterCount >= 3 {
				firstLine = line
				break
			}
		}
	}

	typeLabel, hasType := sectionLabels[sectionType]

	if firstLine != "" {
		runes := []rune(firstLine)
		if len(runes) > 60 {
			firstLine = string(runes[:60]) + "…"
		}
		if hasType && sectionType != "general" {
			return typeLabel + " — " + firstLine
		}
		return firstLine
	}

	if hasType && sectionType != "general" {
		return fmt.Sprintf("%s %d", typeLabel, index+1)
	}
	return fmt.Sprintf("Règles %d", index+1)
}

func truncateRunes(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes])
}

func extractFirstSentence(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	for i, r := range s {
		if r == '.' || r == '!' || r == '?' {
			sentence := strings.TrimSpace(s[:i+1])
			return truncateRunes(sentence, maxRunes)
		}
		if i > 0 && strings.HasPrefix(s[i:], "\n\n") {
			sentence := strings.TrimSpace(s[:i])
			if sentence != "" {
				return truncateRunes(sentence, maxRunes)
			}
		}
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	cut := string(runes[:maxRunes])
	if idx := strings.LastIndexByte(cut, ' '); idx > maxRunes/2 {
		cut = cut[:idx]
	}
	return strings.TrimSpace(cut) + "…"
}

// ──────────────────────────────────────────────────────────────────────────────
// CHUNKER (découpage de texte)
// ──────────────────────────────────────────────────────────────────────────────

const (
	defaultChunkSize = 700
	minChunkSize     = 150
	minParagraphSize = 40
)

type chunk struct {
	Text      string
	PageStart int
	PageEnd   int
	Position  int
}

// paragraph est un paragraphe avec les pages qu'il couvre.
type paragraph struct {
	Text      string
	PageStart int
	PageEnd   int
}

// chunkPages découpe le document page par page : chaque paragraphe garde son
// numéro de page, les chunks couvrent donc une plage de pages exacte.
func chunkPages(pages []pageInfo) []chunk {
	headers := repeatedHeaderLines(pages)

	var paragraphs []paragraph
	totalRunes := 0
	for _, pg := range pages {
		text := cleanPDFArtifacts(pg.Text, headers)
		for _, para := range splitParagraphs(text) {
			paragraphs = append(paragraphs, paragraph{Text: para, PageStart: pg.Number, PageEnd: pg.Number})
			totalRunes += utf8.RuneCountInString(para)
		}
	}
	if len(paragraphs) == 0 {
		return nil
	}

	if totalRunes < minChunkSize {
		texts := make([]string, len(paragraphs))
		for i, para := range paragraphs {
			texts[i] = para.Text
		}
		return []chunk{{
			Text:      strings.Join(texts, "\n\n"),
			PageStart: paragraphs[0].PageStart,
			PageEnd:   paragraphs[len(paragraphs)-1].PageEnd,
		}}
	}

	paragraphs = mergeParagraphs(paragraphs, minParagraphSize)
	paragraphs = mergeOrphanContinuations(paragraphs)

	chunks := make([]chunk, 0, totalRunes/defaultChunkSize+1)
	var current strings.Builder
	current.Grow(defaultChunkSize * 2)
	currentLen := 0
	pageStart, pageEnd := 0, 0
	position := 0

	flush := func() {
		s := strings.TrimSpace(current.String())
		if utf8.RuneCountInString(s) >= minChunkSize && !isLowQualityText(s) {
			chunks = append(chunks, chunk{
				Text:      s,
				PageStart: pageStart,
				PageEnd:   pageEnd,
				Position:  position,
			})
			position++
		}
		current.Reset()
		currentLen = 0
	}

	for _, para := range paragraphs {
		text := strings.TrimSpace(para.Text)
		if text == "" {
			continue
		}
		paraLen := utf8.RuneCountInString(text)
		if paraLen > defaultChunkSize*2 {
			for _, sc := range splitLongParagraph(text, defaultChunkSize) {
				if isLowQualityText(sc) {
					continue
				}
				chunks = append(chunks, chunk{
					Text:      sc,
					PageStart: para.PageStart,
					PageEnd:   para.PageEnd,
					Position:  position,
				})
				position++
			}
			continue
		}

		if currentLen+paraLen > defaultChunkSize && currentLen > 0 {
			flush()
		}

		if currentLen == 0 {
			pageStart = para.PageStart
		} else {
			current.WriteString("\n\n")
		}
		current.WriteString(text)
		currentLen += paraLen
		pageEnd = para.PageEnd
	}

	if currentLen > 0 {
		flush()
	}

	return chunks
}

func splitParagraphs(text string) []string {
	var paragraphs []string
	for _, p := range strings.Split(text, "\n\n") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		lines := strings.Split(p, "\n")
		var joined []string
		for _, l := range lines {
			l = strings.TrimSpace(l)
			if l == "" {
				continue
			}
			if len(joined) == 0 {
				joined = append(joined, l)
				continue
			}
			if isListMarker(l) {
				joined = append(joined, l)
				continue
			}
			prev := joined[len(joined)-1]
			lastRune := lastNonSpace(prev)
			firstRune := firstNonSpace(l)
			if !isSentenceEnd(lastRune) || (firstRune >= 'a' && firstRune <= 'z') {
				joined[len(joined)-1] = prev + " " + l
			} else {
				joined = append(joined, l)
			}
		}
		paragraphs = append(paragraphs, strings.Join(joined, "\n"))
	}
	if len(paragraphs) == 0 {
		p := strings.TrimSpace(text)
		if p != "" {
			paragraphs = []string{p}
		}
	}
	return paragraphs
}

func mergeOrphanContinuations(paragraphs []paragraph) []paragraph {
	if len(paragraphs) == 0 {
		return paragraphs
	}
	result := []paragraph{paragraphs[0]}
	for _, p := range paragraphs[1:] {
		p.Text = strings.TrimSpace(p.Text)
		if p.Text == "" {
			continue
		}
		first := firstNonSpace(p.Text)
		prev := &result[len(result)-1]
		prevLast := lastNonSpace(prev.Text)

		startsLower := first >= 'a' && first <= 'z'
		prevEndsOpen := !isSentenceEnd(prevLast)
		if startsLower && prevEndsOpen {
			prev.Text += " " + p.Text
			prev.PageEnd = p.PageEnd
			continue
		}
		result = append(result, p)
	}
	return result
}

func mergeParagraphs(paragraphs []paragraph, minSize int) []paragraph {
	if len(paragraphs) == 0 {
		return paragraphs
	}
	var result []paragraph
	acc := paragraphs[0]
	for _, p := range paragraphs[1:] {
		if utf8.RuneCountInString(acc.Text) < minSize {
			acc.Text += " " + p.Text
			acc.PageEnd = p.PageEnd
		} else {
			result = append(result, acc)
			acc = p
		}
	}
	if acc.Text != "" {
		result = append(result, acc)
	}
	return result
}

func isListMarker(s string) bool {
	if len(s) == 0 {
		return false
	}
	r := rune(s[0])
	if r == '•' || r == '-' || r == '*' || r == '–' || r == '—' {
		return true
	}
	if r >= '0' && r <= '9' && len(s) > 1 && (s[1] == '.' || s[1] == ')') {
		return true
	}
	return false
}

func lastNonSpace(s string) rune {
	for i := len(s) - 1; i >= 0; i-- {
		r := rune(s[i])
		if r != ' ' && r != '\t' {
			return r
		}
	}
	return 0
}

func firstNonSpace(s string) rune {
	for _, r := range s {
		if r != ' ' && r != '\t' {
			return r
		}
	}
	return 0
}

func isSentenceEnd(r rune) bool {
	return r == '.' || r == '!' || r == '?' || r == ':' || r == ';'
}

func splitLongParagraph(text string, size int) []string {
	runes := []rune(text)
	if len(runes) <= size {
		return []string{text}
	}

	var chunks []string
	start := 0
	for start < len(runes) {
		remaining := len(runes) - start
		if remaining <= size {
			chunk := strings.TrimSpace(string(runes[start:]))
			if chunk != "" {
				chunks = append(chunks, chunk)
			}
			break
		}
		end := start + size

		cutAt := -1
		lookback := start + (size * 2 / 3)
		for i := end; i >= lookback; i-- {
			r := runes[i]
			if r == '.' || r == '!' || r == '?' {
				if i+1 >= len(runes) || runes[i+1] == ' ' || runes[i+1] == '\n' {
					cutAt = i + 1
					break
				}
			}
		}

		if cutAt == -1 {
			for i := end; i >= start+size/2; i-- {
				if runes[i] == ' ' || runes[i] == '\n' {
					cutAt = i
					break
				}
			}
		}
		if cutAt <= start {
			cutAt = end
		}

		chunk := strings.TrimSpace(string(runes[start:cutAt]))
		if chunk != "" {
			chunks = append(chunks, chunk)
		}

		start = cutAt
		for start < len(runes) && (runes[start] == ' ' || runes[start] == '\n') {
			start++
		}
	}
	return chunks
}

func cleanPDFArtifacts(text string, headers map[string]bool) string {
	text = removeLines(text, headers)
	text = removeTOCLines(text)
	for strings.Contains(text, "\n\n\n") {
		text = strings.ReplaceAll(text, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(text)
}

// repeatedHeaderLines repère les lignes courtes répétées sur tout le document
// (en-têtes, pieds de page, mentions de copyright).
func repeatedHeaderLines(pages []pageInfo) map[string]bool {
	freq := make(map[string]int, 64)
	for _, pg := range pages {
		for _, l := range strings.Split(pg.Text, "\n") {
			t := strings.TrimSpace(l)
			if t == "" {
				continue
			}
			nr := utf8.RuneCountInString(t)
			if nr >= 4 && nr <= 80 {
				freq[t]++
			}
		}
	}
	headers := make(map[string]bool)
	for line, count := range freq {
		if count >= 5 {
			headers[line] = true
		}
	}
	return headers
}

func removeLines(text string, toRemove map[string]bool) string {
	if len(toRemove) == 0 {
		return text
	}
	lines := strings.Split(text, "\n")
	kept := lines[:0]
	for _, l := range lines {
		if !toRemove[strings.TrimSpace(l)] {
			kept = append(kept, l)
		}
	}
	return strings.Join(kept, "\n")
}

func removeTOCLines(text string) string {
	lines := strings.Split(text, "\n")
	var kept []string
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if isTOCLine(t) {
			continue
		}
		kept = append(kept, l)
	}
	return strings.Join(kept, "\n")
}

func isTOCLine(s string) bool {
	if len(s) == 0 || utf8.RuneCountInString(s) > 120 {
		return false
	}
	runes := []rune(s)
	n := len(runes)
	end := n - 1
	for end > 0 && runes[end] == ' ' {
		end--
	}
	if runes[end] < '0' || runes[end] > '9' {
		return false
	}
	leader := false
	for _, r := range runes {
		if r == '.' || r == '_' || r == '·' || r == '…' {
			leader = true
			break
		}
	}
	return leader
}

// isLowQualityText checks whether the provided text is considered low quality based on several heuristics.
// It returns true if:
// - the text is empty or contains only whitespace
// - less than 45% of characters are letters
// - more than 60% of tokens (up to first 30) are short words (2 characters or fewer)
// - the text contains fewer than 5 unique words (case-insensitive, non-alphanumeric chars stripped) in a sample of 10+ tokens
// Otherwise, it returns false.
func isLowQualityText(text string) bool {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) == 0 {
		return true
	}

	letters := 0
	for _, r := range runes {
		if unicode.IsLetter(r) {
			letters++
		}
	}
	if float64(letters)/float64(len(runes)) < 0.45 {
		return true
	}

	tokens := strings.Fields(text)
	if len(tokens) == 0 {
		return true
	}
	sample := tokens
	if len(sample) > 30 {
		sample = sample[:30]
	}
	shortCount := 0
	seen := map[string]bool{}
	for _, t := range sample {
		clean := strings.TrimFunc(t, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
		seen[strings.ToLower(clean)] = true
		if utf8.RuneCountInString(clean) <= 2 {
			shortCount++
		}
	}
	if float64(shortCount)/float64(len(sample)) > 0.6 {
		return true
	}
	if len(seen) < 5 && len(sample) >= 10 {
		return true
	}
	return false
}

// ──────────────────────────────────────────────────────────────────────────────
// EXTRACTOR (extraction de texte PDF/TXT)
// ──────────────────────────────────────────────────────────────────────────────

type pageInfo struct {
	Number int
	Text   string
}

func extractText(absPath string) ([]pageInfo, error) {
	ext := strings.ToLower(filepath.Ext(absPath))
	switch ext {
	case ".pdf":
		return extractPDF(absPath)
	case ".txt", ".md":
		return extractTXT(absPath)
	default:
		return nil, fmt.Errorf("format non supporté: %s", ext)
	}
}

func extractTXT(absPath string) ([]pageInfo, error) {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, err
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return nil, nil
	}
	return []pageInfo{{Number: 1, Text: text}}, nil
}

func extractPDF(absPath string) ([]pageInfo, error) {
	cmd := exec.Command("pdftotext", "-enc", "UTF-8", absPath, "-")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("pdftotext: %w (vérifiez que poppler-utils est installé)", err)
	}
	return splitPerPage(string(out)), nil
}

func splitPerPage(text string) []pageInfo {
	rawPages := strings.Split(text, "\f")
	var pages []pageInfo
	for i, p := range rawPages {
		t := cleanPageText(strings.TrimSpace(p))
		if t != "" {
			pages = append(pages, pageInfo{Number: i + 1, Text: t})
		}
	}
	if len(pages) == 0 {
		pages = []pageInfo{{Number: 1, Text: strings.TrimSpace(text)}}
	}
	return pages
}

func cleanPageText(text string) string {
	var hyphenFixed strings.Builder
	lines := strings.Split(text, "\n")
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		if i < len(lines)-1 && strings.HasSuffix(strings.TrimRight(l, " \t"), "-") {
			trimmed := strings.TrimRight(l, " \t")
			nextLine := strings.TrimLeft(lines[i+1], " \t")
			if len(trimmed) > 1 && len(nextLine) > 0 {
				prevRune := rune(trimmed[len(trimmed)-2])
				nextRune := []rune(nextLine)[0]
				if isLetter(prevRune) && isLowerLetter(nextRune) {
					hyphenFixed.WriteString(trimmed[:len(trimmed)-1])
					hyphenFixed.WriteString(nextLine)
					hyphenFixed.WriteString("\n")
					i++
					continue
				}
			}
		}
		hyphenFixed.WriteString(l)
		hyphenFixed.WriteString("\n")
	}
	text = hyphenFixed.String()

	lines = strings.Split(text, "\n")
	var kept []string
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" {
			kept = append(kept, "")
			continue
		}
		if isPageNumberLine(trimmed) {
			continue
		}
		normalized := normalizeSpaces(trimmed)
		kept = append(kept, normalized)
	}

	result := strings.Join(kept, "\n")
	for strings.Contains(result, "\n\n\n") {
		result = strings.ReplaceAll(result, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(result)
}

func isPageNumberLine(s string) bool {
	onlyDigits := true
	hasDigit := false
	for _, r := range s {
		if r >= '0' && r <= '9' {
			hasDigit = true
		} else if r != ' ' && r != '\t' {
			onlyDigits = false
			break
		}
	}
	if onlyDigits && hasDigit && len([]rune(s)) <= 5 {
		return true
	}

	inner := strings.Trim(s, "-–— \t")
	inner = strings.TrimSpace(inner)
	if len(inner) <= 4 {
		allDigits := true
		for _, r := range inner {
			if r < '0' || r > '9' {
				allDigits = false
				break
			}
		}
		if allDigits && len(inner) > 0 {
			return true
		}
	}
	return false
}

func isLetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r > 127
}

func isLowerLetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r > 127 && strings.ToLower(string(r)) == string(r))
}

func normalizeSpaces(s string) string {
	var prev rune
	var sb strings.Builder
	for _, r := range s {
		if r == ' ' || r == '\t' {
			if prev != ' ' {
				sb.WriteRune(' ')
			}
			prev = ' '
		} else {
			sb.WriteRune(r)
			prev = r
		}
	}
	return sb.String()
}
