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
	"time"
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

// FileInfo décrit un fichier uploadé (page admin des fichiers).
type FileInfo struct {
	Game         string    `json:"game"`
	Name         string    `json:"name"`
	Size         int64     `json:"size"`
	Modified     time.Time `json:"modified"`
	RelativePath string    `json:"relative_path"`
	Path         string    `json:"path"`
}

// ListFiles liste les fichiers de tous les jeux (UPLOADS_DIR/{slug}/*).
func ListFiles() ([]*FileInfo, error) {
	files := []*FileInfo{}
	entries, err := os.ReadDir(config.C.UploadsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return files, nil
		}
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		slug := entry.Name()
		gameDir := filepath.Join(config.C.UploadsDir, slug)
		gameFiles, err := os.ReadDir(gameDir)
		if err != nil {
			continue
		}
		for _, file := range gameFiles {
			if file.IsDir() {
				continue
			}
			info, err := file.Info()
			if err != nil {
				continue
			}
			files = append(files, &FileInfo{
				Game:         slug,
				Name:         file.Name(),
				Size:         info.Size(),
				Modified:     info.ModTime(),
				RelativePath: filepath.Join(slug, file.Name()),
				Path:         filepath.Join(gameDir, file.Name()),
			})
		}
	}
	return files, nil
}

// GameFilePath retourne le chemin absolu de UPLOADS_DIR/{slug}/{filename}
// (slug et filename viennent de l'URL). L'erreur enveloppe fs.ErrNotExist si
// le fichier n'existe pas.
func GameFilePath(slug, filename string) (string, error) {
	// Pas de path traversal hors de UPLOADS_DIR
	if strings.Contains(slug, "..") || strings.Contains(filename, "..") {
		return "", fmt.Errorf("invalid path: contains '..'")
	}
	if slug == "" || filename == "" {
		return "", fmt.Errorf("slug and filename are required")
	}
	absPath := filepath.Join(config.C.UploadsDir, slug, filename)
	if _, err := os.Stat(absPath); err != nil {
		return "", fmt.Errorf("file not found: %w", err)
	}
	return absPath, nil
}

// DeleteGameFile supprime UPLOADS_DIR/{slug}/{filename}, puis le dossier du
// jeu s'il est vide.
func DeleteGameFile(slug, filename string) error {
	absPath, err := GameFilePath(slug, filename)
	if err != nil {
		return err
	}
	if err := os.Remove(absPath); err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}
	os.Remove(filepath.Dir(absPath)) // sans effet si le dossier n'est pas vide
	return nil
}
