// domain/service/embedder.go — Interface service pour l'embedding
package service

import (
	"context"
)

// EmbedderService génère des embeddings (vecteurs) à partir de texte.
// Implémentations possibles : ONNX local, API OpenAI, API Cohere, etc.
type EmbedderService interface {
	// Embed génère le vecteur d'une requête (question utilisateur).
	// Le vecteur est normalisé (L2) et prêt pour la recherche cosine.
	Embed(ctx context.Context, text string) ([]float32, error)

	// EmbedPassage génère le vecteur d'un passage de document à indexer.
	// Les modèles asymétriques (E5) encodent requêtes et passages différemment.
	EmbedPassage(ctx context.Context, text string) ([]float32, error)

	// Dimensions retourne la dimension des vecteurs produits (ex: 384 pour E5-small).
	Dimensions() int
}

// ReleasableEmbedder est implémenté par les embedders capables de libérer
// leur modèle de la RAM entre deux utilisations (ex: ONNX local).
// Utiliser via type assertion : if r, ok := svc.(service.ReleasableEmbedder); ok { r.Release() }
type ReleasableEmbedder interface {
	Release()
}
