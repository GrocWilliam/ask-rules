package storage

import "testing"

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
