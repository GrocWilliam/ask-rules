// pipeline/chunker.go — Découpage de texte en chunks sémantiques
package pipeline

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	DefaultChunkSize = 600 // caractères par chunk cible
	MinChunkSize     = 150 // taille minimale pour garder un chunk
	MinParagraphSize = 40  // taille minimale d'un paragraphe autonome
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
	// Nettoyage global : en-têtes/pieds répétés, lignes parasites
	text = cleanPDFArtifacts(text)

	if utf8.RuneCountInString(text) < MinChunkSize {
		return []Chunk{{Text: text, PageStart: 1, PageEnd: len(pages), Position: 0}}
	}

	paragraphs := splitParagraphs(text)
	paragraphs = mergeParagraphs(paragraphs, MinParagraphSize)
	paragraphs = mergeOrphanContinuations(paragraphs)
	var chunks []Chunk
	var current strings.Builder
	position := 0

	flush := func() {
		s := strings.TrimSpace(current.String())
		if utf8.RuneCountInString(s) >= MinChunkSize && !IsLowQualityText(s) {
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
			subChunks := splitLongParagraph(para, DefaultChunkSize)
			for _, sc := range subChunks {
				if IsLowQualityText(sc) {
					continue
				}
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
// Chaque paragraphe interne est nettoyé : les sauts de ligne simples
// entre phrases sont remplacés par un espace (lignes «enveloppées» par pdftotext).
func splitParagraphs(text string) []string {
	var paragraphs []string
	for _, p := range strings.Split(text, "\n\n") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// Rejoindre les retours à la ligne simples qui NE sont pas
		// des césures de liste (ligne commençant par •, -, *, chiffre+.)
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
			// Nouvelle entrée de liste → garder séparé
			if isListMarker(l) {
				joined = append(joined, l)
				continue
			}
			// Continuer la ligne précédente si elle ne se termine pas par
			// de la ponctuation forte ou si la suivante commence par minuscule
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
		// Fallback : pas de double-newline, traiter comme un seul bloc
		p := strings.TrimSpace(text)
		if p != "" {
			paragraphs = []string{p}
		}
	}
	return paragraphs
}

// mergeOrphanContinuations fusionne les paragraphes qui commencent par une
// lettre minuscule (ou un mot de liaison) avec le paragraphe précédent.
// Cela corrige les faux sauts de paragraphe produits par pdftotext quand il
// rencontre un saut de colonne ou de page au milieu d'une phrase.
func mergeOrphanContinuations(paragraphs []string) []string {
	if len(paragraphs) == 0 {
		return paragraphs
	}
	result := []string{paragraphs[0]}
	for _, p := range paragraphs[1:] {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		first := firstNonSpace(p)
		prev := result[len(result)-1]
		prevLast := lastNonSpace(prev)

		// Fusionner si le paragraphe courant commence par une minuscule
		// ET que le précédent ne se termine pas par une ponctuation forte
		startsLower := first >= 'a' && first <= 'z'
		prevEndsOpen := !isSentenceEnd(prevLast)
		if startsLower && prevEndsOpen {
			result[len(result)-1] = prev + " " + p
			continue
		}
		result = append(result, p)
	}
	return result
}

// mergeParagraphs fusionne les paragraphes trop courts avec le suivant.
func mergeParagraphs(paragraphs []string, minSize int) []string {
	if len(paragraphs) == 0 {
		return paragraphs
	}
	var result []string
	acc := paragraphs[0]
	for _, p := range paragraphs[1:] {
		if utf8.RuneCountInString(acc) < minSize {
			acc = acc + " " + p
		} else {
			result = append(result, acc)
			acc = p
		}
	}
	if acc != "" {
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
	// chiffre suivi de . ou )
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

// splitLongParagraph découpe un paragraphe long en préférant les fins de phrase
// (., ?, !) comme points de coupure, puis en fallback au dernier espace.
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

		// Chercher la dernière fin de phrase dans le dernier tiers de la fenêtre
		cutAt := -1
		lookback := start + (size * 2 / 3)
		for i := end; i >= lookback; i-- {
			r := runes[i]
			if r == '.' || r == '!' || r == '?' {
				// S'assurer qu'un espace ou saut suit (fin de phrase réelle)
				if i+1 >= len(runes) || runes[i+1] == ' ' || runes[i+1] == '\n' {
					cutAt = i + 1 // inclure la ponctuation
					break
				}
			}
		}

		// Fallback : dernier espace dans la fenêtre
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

		// Avancer en sautant les espaces de début
		start = cutAt
		for start < len(runes) && (runes[start] == ' ' || runes[start] == '\n') {
			start++
		}
	}
	return chunks
}

// cleanPDFArtifacts effectue un nettoyage global du texte extrait d'un PDF :
//   - Détecte et supprime les lignes d'en-tête/pied répétées (running headers)
//   - Supprime les lignes de navigation de type « ... 12 » (table des matières)
//   - Supprime les lignes ne contenant que de la ponctuation ou des symboles
func cleanPDFArtifacts(text string) string {
	text = removeRepeatedHeaderLines(text)
	text = removeTOCLines(text)
	// Réduire les séquences de 3+ lignes vides → double saut
	for strings.Contains(text, "\n\n\n") {
		text = strings.ReplaceAll(text, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(text)
}

// removeRepeatedHeaderLines supprime les lignes courtes (≤ 80 caractères) qui
// apparaissent 5 fois ou plus dans le document (en-têtes/pieds de page courants).
func removeRepeatedHeaderLines(text string) string {
	lines := strings.Split(text, "\n")
	freq := make(map[string]int, 64)
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if t == "" {
			continue
		}
		nr := utf8.RuneCountInString(t)
		if nr >= 4 && nr <= 80 {
			freq[t]++
		}
	}
	// Construire l'ensemble des lignes à supprimer
	toRemove := make(map[string]bool)
	for line, count := range freq {
		if count >= 5 {
			toRemove[line] = true
		}
	}
	if len(toRemove) == 0 {
		return text
	}
	var sb strings.Builder
	sb.Grow(len(text))
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if toRemove[t] {
			continue
		}
		sb.WriteString(l)
		sb.WriteByte('\n')
	}
	return sb.String()
}

// removeTOCLines supprime les lignes typiques de table des matières :
// « Titre ...... 12 » ou « Titre _____ 5 » ou « Titre 12 » (ligne courte, fin = chiffre).
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

// isTOCLine retourne true si la ligne ressemble à une entrée de table des matières.
func isTOCLine(s string) bool {
	if len(s) == 0 || utf8.RuneCountInString(s) > 120 {
		return false
	}
	// Doit se terminer par des chiffres (éventuellement précédés de points/underscores/espaces)
	runes := []rune(s)
	n := len(runes)
	// Ignorer les espaces finaux
	end := n - 1
	for end > 0 && runes[end] == ' ' {
		end--
	}
	if runes[end] < '0' || runes[end] > '9' {
		return false // ne finit pas par un chiffre
	}
	// Il doit y avoir au moins 3 points consécutifs, tirets bas ou pointillés
	leader := false
	for _, r := range runes {
		if r == '.' || r == '_' || r == '·' || r == '…' {
			leader = true
			break
		}
	}
	return leader
}

// resolvePageNumber tente de trouver la page d'un chunk dans les pages extraites. de trouver la page d'un chunk dans les pages extraites.
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

// IsLowQualityText retourne true si le texte est trop pauvre pour être indexé :
//   - ratio de lettres < 45 % (tableaux de scores, tables des matières, artefacts PDF)
//   - moins de 6 mots distincts sur 20 tokens (très répétitif)
//   - majorité de tokens courts (≤ 2 caractères) — ex : "0-3 ! 0-3 !"
func IsLowQualityText(text string) bool {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) == 0 {
		return true
	}

	// 1. Ratio de lettres
	letters := 0
	for _, r := range runes {
		if unicode.IsLetter(r) {
			letters++
		}
	}
	if float64(letters)/float64(len(runes)) < 0.45 {
		return true
	}

	// 2. Diversité lexicale et longueur des tokens
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
	// Trop de tokens courts (ponctuations isolées, initiales…)
	if float64(shortCount)/float64(len(sample)) > 0.6 {
		return true
	}
	// Trop peu de mots distincts
	if len(seen) < 5 && len(sample) >= 10 {
		return true
	}
	return false
}
