package handler

import (
	"net/http"
	"testing"
)

func TestSniffExtension(t *testing.T) {
	tests := []struct {
		head, name, want string
		wantErr          bool
	}{
		{"%PDF-1.7\n...", "download", ".pdf", false},
		{"%PDF-1.4", "regles.txt", ".pdf", false}, // contenu PDF malgré le nom
		{"# Règles\n\nMise en place", "regles.md", ".md", false},
		{"Règles du jeu", "download", ".txt", false},
		{"<!DOCTYPE html><html>", "page", "", true},
		{"PK\x03\x04", "archive.zip", "", true},
	}
	for _, tt := range tests {
		got, err := sniffExtension([]byte(tt.head), tt.name)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("sniffExtension(%q, %q) = %q, %v ; attendu %q (erreur : %v)", tt.head, tt.name, got, err, tt.want, tt.wantErr)
		}
	}
}

func TestExtractFilenameFromURL(t *testing.T) {
	tests := []struct{ url, disposition, want string }{
		{"https://x.fr/files/regles.pdf?v=2", "", "regles.pdf"},
		{"https://x.fr/download?id=3", "", "download"},
		{"https://x.fr/", "", "downloaded-file"},
		{"https://x.fr/d", `attachment; filename="a.pdf"; filename*=UTF-8''r%C3%A8gles.pdf`, "règles.pdf"},
		{"https://x.fr/d", `attachment; filename="a.pdf"`, "a.pdf"},
	}
	for _, tt := range tests {
		resp := &http.Response{Header: http.Header{}}
		if tt.disposition != "" {
			resp.Header.Set("Content-Disposition", tt.disposition)
		}
		if got := extractFilenameFromURL(tt.url, resp); got != tt.want {
			t.Errorf("extractFilenameFromURL(%q, %q) = %q ; attendu %q", tt.url, tt.disposition, got, tt.want)
		}
	}
}
