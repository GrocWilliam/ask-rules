// pipeline/chunker.go — Découpage de texte en chunks sémantiques
package pipeline

import (
	"strings"
	"unicode/utf8"
)

const (
	DefaultChunkSize    = 600 // caractères par chunk cible
	DefaultChunkOverlap = 80  // chevauchement entre chunks
	MinChunkSize        = 80  // taille minimale pour garder un chunk
)

// Chunk représente un extrait de texte avec métadonnées.
type Chunk struct {
	Text      string
	PageStart int
	PageEnd   int
	Position  int // index dans la liste des chunks du document
}

// ChunkText découpe un texte en chunks avec chevauchement.
func ChunkText(text string, pages []PageInfo) []Chunk {
	if utf8.RuneCountInString(text) < MinChunkSize {
		return []Chunk{{Text: text, PageStart: 1, PageEnd: len(pages), Position: 0}}
	}

	paragraphs := splitParagraphs(text)
	var chunks []Chunk
	var current strings.Builder
	position := 0

	flush := func() {
		s := strings.TrimSpace(current.String())
		if utf8.RuneCountInString(s) >= MinChunkSize {
			pageNum := resolvePageNumber(s, pages)
			chunks = append(chunks, Chunk{
				Text:      s,
				PageStart: pageNum,
				PageEnd:   pageNum,
				Position:  position,
			})
			position++
		}
		current.Reset()
	}

	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		// Si le paragraphe est très long, le subdiviser
		if utf8.RuneCountInString(para) > DefaultChunkSize*2 {
			subChunks := splitLongParagraph(para, DefaultChunkSize, DefaultChunkOverlap)
			for _, sc := range subChunks {
				pageNum := resolvePageNumber(sc, pages)
				chunks = append(chunks, Chunk{
					Text:      sc,
					PageStart: pageNum,
					PageEnd:   pageNum,
					Position:  position,
				})
				position++
			}
			continue
		}

		currentLen := utf8.RuneCountInString(current.String())
		paraLen := utf8.RuneCountInString(para)

		if currentLen+paraLen > DefaultChunkSize && currentLen > 0 {
			flush()
			// Ajouter un overlap depuis la fin précédente
			if len(chunks) > 0 {
				lastText := chunks[len(chunks)-1].Text
				runes := []rune(lastText)
				overlapStart := len(runes) - DefaultChunkOverlap
				if overlapStart < 0 {
					overlapStart = 0
				}
				current.WriteString(string(runes[overlapStart:]))
				current.WriteString("\n\n")
			}
		}

		if current.Len() > 0 {
			current.WriteString("\n\n")
		}
		current.WriteString(para)
	}

	if current.Len() > 0 {
		flush()
	}

	return chunks
}

// splitParagraphs divise un texte en paragraphes (double saut de ligne).
func splitParagraphs(text string) []string {
	var paragraphs []string
	for _, p := range strings.Split(text, "\n\n") {
		p = strings.TrimSpace(p)
		if p != "" {
			paragraphs = append(paragraphs, p)
		}
	}
	if len(paragraphs) == 0 {
		// Fallback : simple saut de ligne
		for _, p := range strings.Split(text, "\n") {
			p = strings.TrimSpace(p)
			if p != "" {
				paragraphs = append(paragraphs, p)
			}
		}
	}
	return paragraphs
}

// splitLongParagraph découpe un paragraphe long par fenêtre glissante sur les mots.
func splitLongParagraph(text string, size, overlap int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}

	var chunks []string
	i := 0
	for i < len(words) {
		var sb strings.Builder
		j := i
		for j < len(words) && utf8.RuneCountInString(sb.String())+len(words[j]) < size {
			if sb.Len() > 0 {
				sb.WriteString(" ")
			}
			sb.WriteString(words[j])
			j++
		}
		if sb.Len() > 0 {
			chunks = append(chunks, sb.String())
		}
		// Avancer en tenant compte du chevauchement
		step := j - i - overlapWords(words[i:j], overlap)
		if step <= 0 {
			step = 1
		}
		i += step
	}
	return chunks
}

// overlapWords retourne le nombre de mots à conserver pour le chevauchement.
func overlapWords(words []string, targetChars int) int {
	count := 0
	chars := 0
	for k := len(words) - 1; k >= 0; k-- {
		chars += len(words[k]) + 1
		if chars >= targetChars {
			break
		}
		count++
	}
	return count
}

// resolvePageNumber tente de trouver la page d'un chunk dans les pages extraites.
func resolvePageNumber(chunk string, pages []PageInfo) int {
	if len(pages) == 0 {
		return 1
	}
	// Cherche la page qui contient le début du chunk
	preview := chunk
	if len(preview) > 100 {
		preview = preview[:100]
	}
	for _, p := range pages {
		if strings.Contains(p.Text, preview) {
			return p.Number
		}
	}
	return pages[0].Number
}
