// storage/storage.go — Gestion des fichiers uploadés
package storage

import (
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"ask-rules-server/config"
)

func GetAbsolutePath(rel string) string {
	return filepath.Join(config.C.UploadsDir, rel)
}

func GetRelativePath(abs string) string {
	base, _ := filepath.Abs(config.C.UploadsDir)
	rel, err := filepath.Rel(base, abs)
	if err != nil {
		return abs
	}
	return rel
}

func SaveUploadedFile(header *multipart.FileHeader, slug string) (string, error) {
	dir := filepath.Join(config.C.UploadsDir, slug)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("mkdir: %w", err)
	}
	filename := sanitizeFilename(header.Filename)
	destPath := filepath.Join(dir, filename)
	src, err := header.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()
	dst, err := os.Create(destPath)
	if err != nil {
		return "", err
	}
	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		return "", err
	}
	return slug + "/" + filename, nil
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

func ReadFile(path string) ([]byte, error) {
	return os.ReadFile(GetAbsolutePath(path))
}

func FileExists(path string) bool {
	_, err := os.Stat(GetAbsolutePath(path))
	return err == nil
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
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
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
	if safe == "" {
		safe = "file"
	}
	return safe + ext
}
