package storage

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"ask-rules-server/internal/infrastructure/config"
)

func TestSanitizeFilename_StaysInDirectory(t *testing.T) {
	cases := map[string]string{
		"../../app/evil.pdf":   "evil.pdf",
		`..\..\app\evil.pdf`:   "evil.pdf",
		"..":                   "file.",
		"règles du jeu.pdf":    "regles_du_jeu.pdf",
		"x.p/df":               "df",
		"rules.pdf\"; foo=bar": "rules.pdf_foo_bar",
	}
	for in, want := range cases {
		if got := sanitizeFilename(in); got != want {
			t.Errorf("sanitizeFilename(%q) = %q, attendu %q", in, got, want)
		}
	}
}

// useUploadsDir fait pointer UPLOADS_DIR vers un dossier temporaire.
func useUploadsDir(t *testing.T) string {
	dir := t.TempDir()
	old := config.C.UploadsDir
	config.C.UploadsDir = dir
	t.Cleanup(func() { config.C.UploadsDir = old })
	return dir
}

func writeFile(t *testing.T, dir, slug, name string) {
	if err := os.MkdirAll(filepath.Join(dir, slug), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, slug, name), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestListFiles(t *testing.T) {
	dir := useUploadsDir(t)
	writeFile(t, dir, "catan", "regles.pdf")
	writeFile(t, dir, "wingspan", "rules.pdf")
	os.WriteFile(filepath.Join(dir, "orphan.txt"), []byte("x"), 0644) // hors dossier de jeu : ignoré

	files, err := ListFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[0].RelativePath != "catan/regles.pdf" || files[1].Game != "wingspan" {
		t.Errorf("ListFiles = %+v", files)
	}
}

func TestListFiles_MissingDir(t *testing.T) {
	config.C.UploadsDir = filepath.Join(useUploadsDir(t), "absent")
	files, err := ListFiles()
	if err != nil || files == nil || len(files) != 0 {
		t.Errorf("ListFiles = %v, %v ; attendu une liste vide", files, err)
	}
}

func TestGameFilePath_Rejects(t *testing.T) {
	dir := useUploadsDir(t)
	writeFile(t, dir, "catan", "regles.pdf")

	for _, c := range [][2]string{{"../etc", "passwd"}, {"catan", "../../etc/passwd"}, {"", "regles.pdf"}, {"catan", ""}} {
		if _, err := GameFilePath(c[0], c[1]); err == nil {
			t.Errorf("GameFilePath(%q, %q) : erreur attendue", c[0], c[1])
		}
	}
	if _, err := GameFilePath("catan", "absent.pdf"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("fichier absent : %v, attendu fs.ErrNotExist", err)
	}
	if p, err := GameFilePath("catan", "regles.pdf"); err != nil || p != filepath.Join(dir, "catan", "regles.pdf") {
		t.Errorf("GameFilePath = %q, %v", p, err)
	}
}

func TestDeleteGameFile_RemovesEmptyGameDir(t *testing.T) {
	dir := useUploadsDir(t)
	writeFile(t, dir, "catan", "a.pdf")
	writeFile(t, dir, "catan", "b.pdf")

	if err := DeleteGameFile("catan", "a.pdf"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "catan")); err != nil {
		t.Fatal("le dossier du jeu, non vide, doit rester")
	}
	if err := DeleteGameFile("catan", "b.pdf"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "catan")); !os.IsNotExist(err) {
		t.Error("le dossier du jeu, vide, doit être supprimé")
	}
}
