// storage/storage.go — Gestion des fichiers uploadés
package storage

import (
	"ask-rules-server/internal/infrastructure/config"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

func GetAbsolutePath(rel string) string {
	return filepath.Join(config.C.UploadsDir, rel)
}

func SaveUploadedFile(header *multipart.FileHeader, slug string) (string, error) {
	src, err := header.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()
	path, _, err := SaveFile(src, header.Filename, slug)
	return path, err
}

// SaveFile écrit src dans UPLOADS_DIR/{slug}/ sous un nom nettoyé et retourne
// le chemin relatif (slug/fichier) et le nombre d'octets écrits.
func SaveFile(src io.Reader, name, slug string) (string, int64, error) {
	dir := filepath.Join(config.C.UploadsDir, slug)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", 0, fmt.Errorf("mkdir: %w", err)
	}
	filename := sanitizeFilename(name)
	destPath := filepath.Join(dir, filename)
	dst, err := os.Create(destPath)
	if err != nil {
		return "", 0, err
	}
	defer dst.Close()
	size, err := io.Copy(dst, src)
	if err != nil {
		os.Remove(destPath)
		return "", size, err
	}
	return slug + "/" + filename, size, nil
}

func DeleteGameFiles(paths []string) {
	for _, p := range paths {
		abs := GetAbsolutePath(p)
		os.Remove(abs)
		dir := filepath.Dir(abs)
		entries, _ := os.ReadDir(dir)
		if len(entries) == 0 {
			os.Remove(dir)
		}
	}
}

func DeleteFile(path string) error {
	return os.Remove(GetAbsolutePath(path))
}

var (
	diacritics = strings.NewReplacer(
		"é", "e", "è", "e", "ê", "e", "ë", "e",
		"à", "a", "â", "a", "ä", "a",
		"î", "i", "ï", "i",
		"ô", "o", "ö", "o",
		"ù", "u", "û", "u", "ü", "u",
		"ç", "c",
		"É", "E", "È", "E", "Ê", "E", "Ë", "E",
		"À", "A", "Â", "A", "Ä", "A",
		"Î", "I", "Ï", "I",
		"Ô", "O", "Ö", "O",
		"Ù", "U", "Û", "U", "Ü", "U",
		"Ç", "C",
	)
	nonAlpha = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)
)

func sanitizeFilename(name string) string {
	// Dernier segment seulement (« ../../x » → « x »), séparateurs Windows compris
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	rawExt := filepath.Ext(name)
	base := strings.TrimSuffix(name, rawExt)
	ext := nonAlpha.ReplaceAllString(rawExt, "_")
	base = diacritics.Replace(base)
	var b strings.Builder
	for _, r := range base {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	safe := nonAlpha.ReplaceAllString(b.String(), "_")
	if strings.Trim(safe, ".") == "" {
		safe = "file"
	}
	return safe + ext
}
