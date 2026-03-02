// pipeline/extractor.go — Extraction de texte (PDF + TXT)
package pipeline

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// PageInfo informations sur une page extraite.
type PageInfo struct {
	Number int
	Text   string
}

// ExtractText extrait le texte d'un fichier (PDF ou TXT).
// Retourne le texte complet et les pages individuelles.
func ExtractText(absPath string) (string, []PageInfo, error) {
	ext := strings.ToLower(filepath.Ext(absPath))
	switch ext {
	case ".pdf":
		return extractPDF(absPath)
	case ".txt", ".md":
		return extractTXT(absPath)
	default:
		return "", nil, fmt.Errorf("format non supporté: %s", ext)
	}
}

func extractTXT(absPath string) (string, []PageInfo, error) {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", nil, err
	}
	text := string(data)
	return text, []PageInfo{{Number: 1, Text: text}}, nil
}

func extractPDF(absPath string) (string, []PageInfo, error) {
	// Sans -layout : pdftotext utilise l'ordre de lecture logique, ce qui donne
	// un texte fluide même pour les documents multi-colonnes (meilleur que le mode
	// « mise en page visuelle » qui interleave les colonnes et génère des espaces parasites).
	cmd := exec.Command("pdftotext", "-enc", "UTF-8", absPath, "-")
	out, err := cmd.Output()
	if err != nil {
		return "", nil, fmt.Errorf("pdftotext: %w (vérifiez que poppler-utils est installé)", err)
	}

	full := string(out)
	pages := splitPerPage(full)

	// Reconstruire le texte complet à partir des pages nettoyées
	var sb strings.Builder
	for i, p := range pages {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(p.Text)
	}
	return sb.String(), pages, nil
}

// splitPerPage découpe un texte PDF en pages (séparateur form-feed \f) et
// nettoie chaque page : numéros de page isolés, en-têtes/pieds courts répétés,
// traits d'union de fin de ligne, espaces multiples.
func splitPerPage(text string) []PageInfo {
	rawPages := strings.Split(text, "\f")
	var pages []PageInfo
	for i, p := range rawPages {
		t := cleanPageText(strings.TrimSpace(p))
		if t != "" {
			pages = append(pages, PageInfo{Number: i + 1, Text: t})
		}
	}
	if len(pages) == 0 {
		pages = []PageInfo{{Number: 1, Text: strings.TrimSpace(text)}}
	}
	return pages
}

// cleanPageText nettoie le texte brut d'une page PDF :
//   - Supprime les lignes qui ne contiennent que des chiffres (numéros de page)
//   - Supprime les lignes de type "- 12 -" ou "— 12 —" (numéro encadré de tirets)
//   - Rejoint les coupures de mot en fin de ligne ("diffé-\nrents" → "différents")
//   - Normalise les espaces multiples en un seul espace
func cleanPageText(text string) string {
	// 1. Rejoindre les traits d'union de coupure de mot (césure typographique)
	//    "Reconsti-\ntuer" → "Reconstituer"
	//    On détecte: lettre + tiret + \n + lettre (minuscule = continuation)
	var hyphenFixed strings.Builder
	lines := strings.Split(text, "\n")
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		if i < len(lines)-1 && strings.HasSuffix(strings.TrimRight(l, " \t"), "-") {
			trimmed := strings.TrimRight(l, " \t")
			nextLine := strings.TrimLeft(lines[i+1], " \t")
			if len(trimmed) > 1 && len(nextLine) > 0 {
				prevRune := rune(trimmed[len(trimmed)-2]) // char before hyphen
				nextRune := []rune(nextLine)[0]
				// Rejoint si : char préc. est une lettre, char suivant est minuscule
				if isLetter(prevRune) && isLowerLetter(nextRune) {
					hyphenFixed.WriteString(trimmed[:len(trimmed)-1]) // sans le tiret
					hyphenFixed.WriteString(nextLine)
					hyphenFixed.WriteString("\n")
					i++ // sauter la ligne suivante déjà consommée
					continue
				}
			}
		}
		hyphenFixed.WriteString(l)
		hyphenFixed.WriteString("\n")
	}
	text = hyphenFixed.String()

	// 2. Supprimer les lignes vides ou contenant uniquement des chiffres / formats de n° de page
	lines = strings.Split(text, "\n")
	var kept []string
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" {
			kept = append(kept, "")
			continue
		}
		if isPageNumberLine(trimmed) {
			continue // supprimer
		}
		// Normaliser les espaces internes multiples (artéfact de -layout residuel)
		normalized := normalizeSpaces(trimmed)
		kept = append(kept, normalized)
	}

	// Reconstruire en réduisant les séquences de 3+ lignes vides → 1 ligne vide
	result := strings.Join(kept, "\n")
	for strings.Contains(result, "\n\n\n") {
		result = strings.ReplaceAll(result, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(result)
}

// isPageNumberLine retourne true si la ligne contient uniquement un numéro de page.
//   - "12", "  42  ", "- 12 -", "— 42 —", "· 5 ·"
func isPageNumberLine(s string) bool {
	// Uniquement des chiffres (et espaces)
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

	// Format "- N -" ou "— N —"
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

// normalizeSpaces remplace les séquences d'espaces multiples par un seul espace.
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
