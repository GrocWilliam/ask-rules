package handler

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestImportStream_SendsUntilClosed(t *testing.T) {
	w := httptest.NewRecorder()
	stream, ok := startImportStream(w, httptest.NewRequest("POST", "/api/import", nil))
	if !ok {
		t.Fatal("flux non ouvert")
	}
	stream.send("progress", map[string]interface{}{"current": 1})
	stream.close()
	stream.sendError("après la fin du handler")

	body := w.Body.String()
	if w.Header().Get("Content-Type") != "text/event-stream" {
		t.Errorf("Content-Type inattendu : %q", w.Header().Get("Content-Type"))
	}
	if !strings.Contains(body, `"type":"progress"`) || !strings.Contains(body, `"current":1`) {
		t.Errorf("événement manquant : %q", body)
	}
	if strings.Contains(body, "après la fin") {
		t.Errorf("rien ne doit être écrit après close : %q", body)
	}
}
