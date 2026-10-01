// cmd/eval/main.go — Évaluation de la qualité du retrieval RAG
//
// Mesure, sur un jeu de questions annotées, si les sections renvoyées par le
// retriever contiennent la réponse attendue.
//
// Usage (depuis server/) :
//
//	go run ./cmd/eval -index              # (ré)indexe les jeux du dataset puis évalue
//	go run ./cmd/eval                     # évalue sur l'index existant
//	go run ./cmd/eval -v                  # détaille les questions en échec
//
// Le dataset (eval/questions.json) associe chaque question à un ou plusieurs
// extraits du livret de règles. Une section est considérée pertinente si elle
// contient l'un de ces extraits (comparaison insensible à la casse, aux espaces
// et aux apostrophes typographiques) : le dataset reste valide quel que soit le
// découpage en chunks.
//
// Attention : -index supprime et recrée les sections des jeux du dataset.
// Utilisez une base dédiée (DATABASE_URL).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"ask-rules-server/internal/application/usecase"
	"ask-rules-server/internal/domain/entity"
	"ask-rules-server/internal/domain/service"
	"ask-rules-server/internal/infrastructure/config"
	"ask-rules-server/internal/infrastructure/persistence/db"
	postgresRepo "ask-rules-server/internal/infrastructure/persistence/postgres"
	infraService "ask-rules-server/internal/infrastructure/service"
)

type dataset struct {
	Games []struct {
		Name  string   `json:"name"`
		Files []string `json:"files"` // chemins relatifs à UPLOADS_DIR
	} `json:"games"`
	Questions []question `json:"questions"`
}

type question struct {
	Game     string   `json:"game"`
	Question string   `json:"question"`
	Expected []string `json:"expected"` // extraits dont l'un doit figurer dans une section renvoyée
}

func main() {
	datasetPath := flag.String("dataset", "eval/questions.json", "fichier du dataset")
	doIndex := flag.Bool("index", false, "(ré)indexer les jeux du dataset avant l'évaluation")
	limit := flag.Int("k", 6, "nombre de sections renvoyées (identique à /api/ask)")
	verbose := flag.Bool("v", false, "afficher le détail des questions en échec")
	flag.Parse()

	if err := config.Load(); err != nil {
		log.Fatalf("config : %v", err)
	}
	ds, err := loadDataset(*datasetPath)
	if err != nil {
		log.Fatalf("dataset : %v", err)
	}

	ctx := context.Background()
	if err := db.Connect(); err != nil {
		log.Fatalf("connexion base : %v", err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		log.Fatalf("migration : %v", err)
	}

	gameRepo := postgresRepo.NewGameRepository(db.Pool)
	sectionRepo := postgresRepo.NewSectionRepository(db.Pool)
	embedder := infraService.NewONNXEmbedder()
	retriever := infraService.NewHybridRetriever(sectionRepo, embedder)

	if *doIndex {
		pipeline := infraService.NewPipeline(gameRepo, sectionRepo, embedder)
		for _, g := range ds.Games {
			start := time.Now()
			if err := indexGame(ctx, gameRepo, sectionRepo, pipeline, g.Name, g.Files); err != nil {
				log.Fatalf("indexation %s : %v", g.Name, err)
			}
			log.Printf("indexé %-20s en %s", g.Name, time.Since(start).Round(time.Millisecond))
		}
	}

	// Pages de référence de chaque jeu, extraites indépendamment du pipeline
	gamePages := map[string][]string{}
	for _, g := range ds.Games {
		for _, f := range g.Files {
			pages, err := referencePages(filepath.Join(config.C.UploadsDir, f))
			if err != nil {
				log.Fatalf("pages de référence %s : %v", f, err)
			}
			gamePages[g.Name] = append(gamePages[g.Name], pages...)
		}
	}

	gameIDs := map[string]string{}
	for _, g := range ds.Games {
		game, err := gameRepo.FindByName(ctx, g.Name)
		if err != nil {
			log.Fatalf("jeu %q introuvable (lancer avec -index ?) : %v", g.Name, err)
		}
		gameIDs[g.Name] = game.ID
	}

	var hit1, hit3, hitK, vecHitK, empty int
	var pageChecked, pageCorrect int
	var mrr float64
	var totalChars int
	var totalLatency time.Duration
	var failures []string

	for _, q := range ds.Questions {
		start := time.Now()
		sections, err := retriever.Search(ctx, &service.SearchRequest{
			GameID:   gameIDs[q.Game],
			Question: q.Question,
			Limit:    *limit,
		})
		totalLatency += time.Since(start)
		if err != nil {
			log.Fatalf("recherche %q : %v", q.Question, err)
		}
		if len(sections) == 0 {
			empty++
		}

		rank := 0
		for i, s := range sections {
			totalChars += len(s.Text)
			if rank == 0 && matches(s.Text, q.Expected) {
				rank = i + 1
				// La page attribuée à la section contient-elle l'extrait attendu ?
				if want := expectedPages(gamePages[q.Game], q.Expected); len(want) > 0 {
					pageChecked++
					if pageInRange(want, s.PageStart, s.PageEnd) {
						pageCorrect++
					} else if *verbose {
						failures = append(failures, fmt.Sprintf("✗ page [%s] %s : pages attendues %v, section p.%s",
							q.Game, q.Question, want, formatRange(s.PageStart, s.PageEnd)))
					}
				}
			}
		}
		switch {
		case rank == 0:
			failures = append(failures, describeFailure(q, sections))
		case rank == 1:
			hit1++
			fallthrough
		case rank <= 3:
			hit3++
			fallthrough
		default:
			hitK++
			mrr += 1 / float64(rank)
		}

		// Diagnostic : la recherche vectorielle seule trouve-t-elle la réponse ?
		vec, err := embedder.Embed(ctx, q.Question)
		if err == nil {
			f64 := make([]float64, len(vec))
			for i, f := range vec {
				f64[i] = float64(f)
			}
			vr, err := sectionRepo.VectorSearch(ctx, gameIDs[q.Game], f64, *limit)
			if err == nil {
				for _, s := range vr {
					if matches(s.Text, q.Expected) {
						vecHitK++
						break
					}
				}
			}
		}
	}

	n := float64(len(ds.Questions))
	pct := func(v int) string { return fmt.Sprintf("%5.1f%%", 100*float64(v)/n) }
	fmt.Println()
	fmt.Printf("Questions            : %d (%d jeux)\n", len(ds.Questions), len(ds.Games))
	fmt.Printf("Hit@1                : %s\n", pct(hit1))
	fmt.Printf("Hit@3                : %s\n", pct(hit3))
	fmt.Printf("Hit@%d                : %s\n", *limit, pct(hitK))
	fmt.Printf("MRR@%d                : %.3f\n", *limit, mrr/n)
	fmt.Printf("Hit@%d vectoriel seul : %s\n", *limit, pct(vecHitK))
	fmt.Printf("Sans résultat        : %d\n", empty)
	if pageChecked > 0 {
		fmt.Printf("Page correcte        : %5.1f%% (%d/%d sections trouvées)\n",
			100*float64(pageCorrect)/float64(pageChecked), pageCorrect, pageChecked)
	}
	fmt.Printf("Contexte moyen       : %d caractères\n", totalChars/len(ds.Questions))
	fmt.Printf("Latence moyenne      : %s\n", (totalLatency / time.Duration(len(ds.Questions))).Round(time.Millisecond))

	if *verbose && len(failures) > 0 {
		fmt.Println("\nÉchecs :")
		for _, f := range failures {
			fmt.Println(f)
		}
	}
}

func loadDataset(path string) (*dataset, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var ds dataset
	if err := json.Unmarshal(data, &ds); err != nil {
		return nil, err
	}
	if len(ds.Questions) == 0 {
		return nil, fmt.Errorf("aucune question")
	}
	return &ds, nil
}

// indexGame supprime les sections du jeu puis le réindexe avec le pipeline courant.
func indexGame(ctx context.Context, gameRepo interface {
	FindByName(context.Context, string) (*entity.Game, error)
	Save(context.Context, *entity.Game) error
}, sectionRepo interface {
	DeleteByGameID(context.Context, string) error
}, pipeline usecase.PipelineService, name string, files []string) error {
	game, err := gameRepo.FindByName(ctx, name)
	if err != nil {
		game = &entity.Game{ID: "eval-" + strings.ToLower(strings.ReplaceAll(name, " ", "-")), Name: name}
	}
	game.Stats = map[string]interface{}{"files": files}
	if err := gameRepo.Save(ctx, game); err != nil {
		return err
	}
	if err := sectionRepo.DeleteByGameID(ctx, game.ID); err != nil {
		return err
	}
	return pipeline.Process(ctx, &usecase.ImportOptions{GameName: name, FilePaths: files})
}

var normalizer = strings.NewReplacer("’", "'", "‘", "'", "«", "\"", "»", "\"", "“", "\"", "”", "\"", " ", " ")

func normalizeText(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(normalizer.Replace(s))), " ")
}

func matches(text string, expected []string) bool {
	t := normalizeText(text)
	for _, e := range expected {
		if strings.Contains(t, normalizeText(e)) {
			return true
		}
	}
	return false
}

func describeFailure(q question, sections []*entity.ScoredSection) string {
	var b strings.Builder
	fmt.Fprintf(&b, "✗ [%s] %s\n    attendu : %q\n", q.Game, q.Question, q.Expected)
	for i, s := range sections {
		preview := []rune(strings.Join(strings.Fields(s.Text), " "))
		if len(preview) > 90 {
			preview = preview[:90]
		}
		fmt.Fprintf(&b, "    %d. (%.2f) %s…\n", i+1, s.Score, string(preview))
	}
	return b.String()
}

// referencePages renvoie le texte normalisé de chaque page (index i = page i+1).
func referencePages(path string) ([]string, error) {
	var raw string
	if strings.EqualFold(filepath.Ext(path), ".pdf") {
		out, err := exec.Command("pdftotext", "-enc", "UTF-8", path, "-").Output()
		if err != nil {
			return nil, err
		}
		raw = string(out)
	} else {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		raw = string(data)
	}
	pages := strings.Split(raw, "\f")
	for i, p := range pages {
		pages[i] = normalizeText(p)
	}
	return pages, nil
}

// expectedPages renvoie les pages contenant l'un des extraits attendus.
func expectedPages(pages []string, expected []string) []int {
	var result []int
	for i, p := range pages {
		for _, e := range expected {
			if strings.Contains(p, normalizeText(e)) {
				result = append(result, i+1)
				break
			}
		}
	}
	return result
}

func pageInRange(want []int, start, end *int) bool {
	if start == nil {
		return false
	}
	last := *start
	if end != nil && *end > last {
		last = *end
	}
	for _, p := range want {
		if p >= *start && p <= last {
			return true
		}
	}
	return false
}

func formatRange(start, end *int) string {
	if start == nil {
		return "?"
	}
	if end != nil && *end > *start {
		return fmt.Sprintf("%d-%d", *start, *end)
	}
	return fmt.Sprint(*start)
}
