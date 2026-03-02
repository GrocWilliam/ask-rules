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
	// Essaie pdftoppm ou pdftotext
	cmd := exec.Command("pdftotext", "-layout", "-enc", "UTF-8", absPath, "-")
	out, err := cmd.Output()
	if err != nil {
		return "", nil, fmt.Errorf("pdftotext: %w (vérifiez que poppler-utils est installé)", err)
	}

	full := string(out)
	pages := splitPerPage(full)
	return full, pages, nil
}

// splitPerPage découpe un texte PDF en pages (séparateur form-feed \f).
func splitPerPage(text string) []PageInfo {
	rawPages := strings.Split(text, "\f")
	var pages []PageInfo
	for i, p := range rawPages {
		t := strings.TrimSpace(p)
		if t != "" {
			pages = append(pages, PageInfo{Number: i + 1, Text: t})
		}
	}
	if len(pages) == 0 {
		pages = []PageInfo{{Number: 1, Text: strings.TrimSpace(text)}}
	}
	return pages
}
