// infrastructure/service/llm_health.go — Suivi d'un serveur LLM mis en veille
//
// Un LLM auto-hébergé (llama.cpp sur Railway…) peut être mis en veille quand il
// est inactif : le réveil (démarrage + chargement du modèle) prend du temps.
// Plutôt que de faire attendre l'utilisateur, on vérifie l'endpoint de santé
// avant chaque appel ; s'il ne répond pas, on réveille le serveur en arrière-plan
// et la question part sur le secours.
package service

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"
)

const (
	// healthFreshness : durée pendant laquelle un succès récent évite un nouveau contrôle
	healthFreshness = 30 * time.Second
	// healthCheckTimeout : délai d'un contrôle avant un appel (court : l'utilisateur attend)
	healthCheckTimeout = 3 * time.Second
	// wakePollTimeout / wakePollInterval : contrôles répétés pendant le réveil
	wakePollTimeout  = 10 * time.Second
	wakePollInterval = 3 * time.Second
)

type healthState struct {
	name        string
	url         string
	wakeTimeout time.Duration
	client      *http.Client

	mu     sync.Mutex
	lastOK time.Time
	waking bool
}

func newHealthState(name, url string, wakeTimeout time.Duration, client *http.Client) *healthState {
	if url == "" {
		return nil
	}
	return &healthState{name: name, url: url, wakeTimeout: wakeTimeout, client: client}
}

// check interroge l'endpoint de santé (llama.cpp répond 503 tant que le modèle charge).
func (h *healthState) check(ctx context.Context, timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.url, nil)
	if err != nil {
		return false
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	h.markOK()
	return true
}

func (h *healthState) markOK() {
	h.mu.Lock()
	h.lastOK = time.Now()
	h.mu.Unlock()
}

// markDown oublie le dernier succès : le prochain appel revérifiera la santé.
func (h *healthState) markDown() {
	h.mu.Lock()
	h.lastOK = time.Time{}
	h.mu.Unlock()
}

func (h *healthState) fresh() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return time.Since(h.lastOK) < healthFreshness
}

// ready indique si le serveur est prêt ; sinon déclenche son réveil en arrière-plan.
func (h *healthState) ready(ctx context.Context) bool {
	if h.fresh() || h.check(ctx, healthCheckTimeout) {
		return true
	}
	h.wake()
	return false
}

// wake réveille le serveur en arrière-plan (une seule boucle à la fois) :
// contrôles répétés jusqu'à ce qu'il réponde ou que wakeTimeout soit écoulé.
func (h *healthState) wake() {
	h.mu.Lock()
	if h.waking {
		h.mu.Unlock()
		return
	}
	h.waking = true
	h.mu.Unlock()

	log.Printf("[INFO] LLM - %s ne répond pas, réveil en cours", h.name)
	go func() {
		defer func() {
			h.mu.Lock()
			h.waking = false
			h.mu.Unlock()
		}()
		start := time.Now()
		for time.Since(start) < h.wakeTimeout {
			if h.check(context.Background(), wakePollTimeout) {
				log.Printf("[INFO] LLM - %s réveillé en %s", h.name, time.Since(start).Round(time.Second))
				return
			}
			time.Sleep(wakePollInterval)
		}
		log.Printf("[WARN] LLM - %s toujours indisponible après %s", h.name, h.wakeTimeout)
	}()
}

// waitReady attend que le serveur soit prêt (ou l'annulation du contexte).
func (h *healthState) waitReady(ctx context.Context) error {
	if h.ready(ctx) {
		return nil
	}
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			if h.fresh() {
				return nil
			}
		}
	}
}
