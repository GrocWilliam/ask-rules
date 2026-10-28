// cmd/eval/main.go — Évaluation de la qualité du RAG : retrieval puis réponse du LLM
//
// Mesure, sur un jeu de questions annotées :
//   - retrieval : les sections trouvées, puis celles envoyées au LLM (après le
//     budget de contexte), contiennent-elles la réponse attendue ?
//   - LLM (-llm) : la réponse contient-elle les faits attendus, cite-t-elle une
//     bonne page, refuse-t-elle à tort ou au contraire invente-t-elle une règle ?
//
// Usage (depuis server/) :
//
//	go run ./cmd/eval -index              # (ré)indexe les jeux du dataset puis évalue
//	go run ./cmd/eval                     # évalue le retrieval sur l'index existant
//	go run ./cmd/eval -llm                # évalue aussi les réponses du LLM (LLM_*)
//	go run ./cmd/eval -llm -run paradoxe  # seulement les questions qui contiennent « paradoxe »
//	go run ./cmd/eval -v -out eval.json   # détaille les échecs, enregistre les résultats
//	go run ./cmd/eval -model ../models/multilingual-e5-base  # compare un autre modèle d'embedding
//
// Le dataset (eval/questions.json) associe chaque question à :
//   - expected : extraits du livret dont l'un doit figurer dans une section
//     (comparaison insensible à la casse, aux espaces et aux apostrophes
//     typographiques) : le dataset reste valide quel que soit le découpage en
//     chunks. Vide : la réponse n'est pas dans le livret, le LLM doit refuser.
//   - answer : faits que la réponse du LLM doit contenir, chacun avec ses
//     variantes séparées par « | » (« 3|trois »). Un fait est cherché en mot
//     entier : « 3 » ne trouve pas « 13 ».
//   - history (optionnel) : échanges précédents, pour une question de suivi.
//
// La recherche et le contexte sont ceux de /api/ask (usecase.RetrieveContext) ;
// le LLM est appelé directement, sans cache.
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
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"ask-rules-server/internal/application/usecase"
	"ask-rules-server/internal/domain/entity"
	"ask-rules-server/internal/domain/repository"
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
	Game     string            `json:"game"`
	Question string            `json:"question"`
	History  []entity.ChatTurn `json:"history,omitempty"`
	Expected []string          `json:"expected"`         // extraits dont l'un doit figurer dans une section renvoyée
	Answer   []string          `json:"answer,omitempty"` // faits attendus dans la réponse, variantes séparées par « | »
}

// outOfBook : la réponse n'est pas dans le livret, le LLM doit le dire.
func (q question) outOfBook() bool { return len(q.Expected) == 0 }

// result : mesures d'une question, enregistrées par -out.
type result struct {
	Game     string `json:"game"`
	Question string `json:"question"`

	// Retrieval
	Rank          int   `json:"rank"`                     // rang de la 1re candidate pertinente (0 : absente)
	InContext     bool  `json:"in_context"`               // la réponse est dans le contexte envoyé au LLM
	VectorHit     bool  `json:"vector_hit"`               // trouvée par la recherche vectorielle seule
	PageOK        *bool `json:"page_ok,omitempty"`        // la section trouvée porte une page attendue
	ExpectedPages []int `json:"expected_pages,omitempty"` // pages du livret qui contiennent l'extrait
	ContextChars  int   `json:"context_chars"`
	SearchMs      int64 `json:"search_ms"`

	// LLM (-llm)
	Answer       string   `json:"answer,omitempty"`
	Model        string   `json:"model,omitempty"` // modèle qui a répondu (le secours si le principal dort)
	Error        string   `json:"error,omitempty"`
	FactsFound   int      `json:"facts_found,omitempty"`
	FactsTotal   int      `json:"facts_total,omitempty"`
	Missing      []string `json:"missing,omitempty"`
	Refused      bool     `json:"refused,omitempty"`
	CitedPages   []int    `json:"cited_pages,omitempty"`
	InventedPage []int    `json:"invented_pages,omitempty"` // citées mais absentes du contexte
	Words        int      `json:"words,omitempty"`
	FirstTokenMs int64    `json:"first_token_ms,omitempty"`
	TotalMs      int64    `json:"total_ms,omitempty"`
	Tokens       int      `json:"tokens,omitempty"`
}

// candidateLimit : nombre de candidates demandées par /api/ask (usecase.search).
const candidateLimit = 6

// maxWords : longueur max d'une réponse demandée par le prompt (liste comprise).
const maxWords = 150

func main() {
	datasetPath := flag.String("dataset", "eval/questions.json", "fichier du dataset")
	doIndex := flag.Bool("index", false, "(ré)indexer les jeux du dataset avant l'évaluation")
	withLLM := flag.Bool("llm", false, "évaluer aussi les réponses du LLM (LLM_* doit être configuré)")
	filter := flag.String("run", "", "n'évaluer que les questions qui correspondent à cette regexp")
	outPath := flag.String("out", "", "enregistrer le détail des résultats dans ce fichier JSON")
	verbose := flag.Bool("v", false, "afficher le détail des questions en échec")
	modelPath := flag.String("model", "", "modèle d'embedding à comparer : sections vectorisées en mémoire, sans réindexer")
	minScore := flag.Float64("min-score", 0, "seuil de similarité vectorielle (défaut : VECTOR_MIN_SCORE ; négatif : aucun)")
	flag.Parse()

	if err := config.Load(); err != nil {
		log.Fatalf("config : %v", err)
	}
	if *modelPath != "" {
		if *doIndex {
			log.Fatal("-model et -index sont incompatibles : -model ne modifie pas la base")
		}
		config.C.ModelPath = *modelPath
	}
	if *minScore != 0 {
		config.C.VectorMinScore = *minScore
	}
	ds, err := loadDataset(*datasetPath, *filter)
	if err != nil {
		log.Fatalf("dataset : %v", err)
	}

	ctx := context.Background()
	if err := db.Connect(); err != nil {
		log.Fatalf("connexion base : %v", err)
	}
	defer db.Close()

	gameRepo := postgresRepo.NewGameRepository(db.Pool)
	sectionRepo := postgresRepo.NewSectionRepository(db.Pool)
	embedder := infraService.NewONNXEmbedder()
	llm := infraService.NewLLM()

	if *withLLM {
		if config.C.LLM.BaseURL == "" {
			log.Fatal("-llm : aucun LLM configuré (LLM_BASE_URL)")
		}
		llm.Warmup() // réveille le LLM pendant l'indexation et le retrieval
	}

	// Sans -index, l'évaluation n'écrit rien en base
	if *doIndex {
		if err := db.Migrate(ctx); err != nil {
			log.Fatalf("migration : %v", err)
		}
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

	// Avec -model, la recherche vectorielle utilise les vecteurs du modèle comparé
	var vectorRepo repository.SectionRepository = sectionRepo
	if *modelPath != "" {
		ids := make([]string, 0, len(gameIDs))
		for _, id := range gameIDs {
			ids = append(ids, id)
		}
		vectorRepo, err = newMemoryVectors(ctx, db.Pool, sectionRepo, embedder, ids)
		if err != nil {
			log.Fatalf("-model : %v", err)
		}
	}
	retriever := infraService.NewHybridRetriever(vectorRepo, embedder)
	ask := usecase.NewAskQuestionUseCase(gameRepo, retriever, llm, nil, nil)

	var results []*result
	var failures []string
	for i, q := range ds.Questions {
		r, retrieved, problems := evalRetrieval(ctx, ask, embedder, vectorRepo, gameIDs[q.Game], gamePages[q.Game], q)
		failures = append(failures, problems...)
		if *withLLM {
			evalAnswer(ctx, llm, q, retrieved, r)
			log.Printf("[%d/%d] %s — %s", i+1, len(ds.Questions), q.Question, llmSummary(q, r))
			if failure := describeAnswerFailure(q, r); failure != "" {
				failures = append(failures, failure)
			}
		}
		results = append(results, r)
	}

	printRetrieval(ds, results)
	if *withLLM {
		printLLM(ds, results)
	}

	if *verbose && len(failures) > 0 {
		fmt.Println("\nÉchecs :")
		for _, f := range failures {
			fmt.Println(f)
		}
	}

	if *outPath != "" {
		data, _ := json.MarshalIndent(results, "", "  ")
		if err := os.WriteFile(*outPath, data, 0o644); err != nil {
			log.Fatalf("écriture %s : %v", *outPath, err)
		}
		fmt.Printf("\nRésultats enregistrés dans %s\n", *outPath)
	}
}

// evalRetrieval mesure la recherche d'une question ; failures décrit ses échecs.
func evalRetrieval(ctx context.Context, ask *usecase.AskQuestionUseCase, embedder service.EmbedderService,
	sectionRepo interface {
		VectorSearch(context.Context, string, []float64, int) ([]*entity.ScoredSection, error)
	}, gameID string, pages []string, q question) (r *result, retrieved *usecase.RetrievedContext, failures []string) {
	r = &result{Game: q.Game, Question: q.Question}

	start := time.Now()
	retrieved, err := ask.RetrieveContext(ctx, gameID, q.Question, q.History)
	r.SearchMs = time.Since(start).Milliseconds()
	if err != nil {
		log.Fatalf("recherche %q : %v", q.Question, err)
	}
	r.ContextChars = utf8.RuneCountInString(retrieved.Text)
	if q.outOfBook() {
		return r, retrieved, nil
	}

	r.ExpectedPages = expectedPages(pages, q.Expected)
	for i, s := range retrieved.Candidates {
		if matches(s.Text, q.Expected) {
			r.Rank = i + 1
			// La page attribuée à la section contient-elle l'extrait attendu ?
			if len(r.ExpectedPages) > 0 {
				ok := pageInRange(r.ExpectedPages, s.PageStart, s.PageEnd)
				r.PageOK = &ok
				if !ok {
					failures = append(failures, fmt.Sprintf("✗ page [%s] %s : pages attendues %v, section p.%s",
						q.Game, q.Question, r.ExpectedPages, formatRange(s.PageStart, s.PageEnd)))
				}
			}
			break
		}
	}
	for _, s := range retrieved.Sections {
		if matches(s.Text, q.Expected) {
			r.InContext = true
			break
		}
	}
	switch {
	case r.Rank == 0:
		failures = append(failures, describeFailure(q, retrieved.Candidates))
	case !r.InContext:
		failures = append(failures, fmt.Sprintf("✗ budget [%s] %s : trouvée au rang %d mais écartée du contexte (%d caractères)",
			q.Game, q.Question, r.Rank, r.ContextChars))
	}

	// Diagnostic : la recherche vectorielle seule trouve-t-elle la réponse ?
	if vec, err := embedder.Embed(ctx, q.Question); err == nil {
		f64 := make([]float64, len(vec))
		for i, f := range vec {
			f64[i] = float64(f)
		}
		if vr, err := sectionRepo.VectorSearch(ctx, gameID, f64, candidateLimit); err == nil {
			for _, s := range vr {
				if matches(s.Text, q.Expected) {
					r.VectorHit = true
					break
				}
			}
		}
	}
	return r, retrieved, failures
}

// evalAnswer interroge le LLM avec le contexte de la question et note sa réponse.
func evalAnswer(ctx context.Context, llm service.LLMService, q question, retrieved *usecase.RetrievedContext, r *result) {
	start := time.Now()
	var firstToken time.Duration
	resp, err := llm.Query(ctx, q.Question, retrieved.Text, q.History, func(string) {
		if firstToken == 0 {
			firstToken = time.Since(start)
		}
	})
	r.TotalMs = time.Since(start).Milliseconds()
	r.FirstTokenMs = firstToken.Milliseconds()
	if err != nil {
		r.Error = err.Error()
		return
	}
	if resp.TokensUsed != nil {
		r.Tokens = resp.TokensUsed.Total
	}

	r.Answer = resp.Answer
	r.Model = resp.Model
	answer := normalizeAnswer(resp.Answer)
	r.Words = len(strings.Fields(answer))
	r.Refused = strings.Contains(answer, "je ne trouve pas")
	// Sans les pages citées : « (p. 3) » ne doit pas valider le fait « 3 »
	withoutPages := normalizeAnswer(pageRef.ReplaceAllString(resp.Answer, ""))
	r.FactsTotal = len(q.Answer)
	for _, fact := range q.Answer {
		if factFound(withoutPages, fact) {
			r.FactsFound++
		} else {
			r.Missing = append(r.Missing, fact)
		}
	}
	r.CitedPages = citedPages(resp.Answer)
	for _, p := range r.CitedPages {
		if !pageInSections(p, retrieved.Sections) {
			r.InventedPage = append(r.InventedPage, p)
		}
	}
}

// complete : la réponse contient tous les faits attendus.
func (r *result) complete() bool { return r.Error == "" && r.FactsFound == r.FactsTotal }

// citesExpectedPage : la réponse cite une page qui contient l'extrait attendu.
func (r *result) citesExpectedPage() bool {
	for _, p := range r.CitedPages {
		if slices.Contains(r.ExpectedPages, p) {
			return true
		}
	}
	return false
}

func llmSummary(q question, r *result) string {
	switch {
	case r.Error != "":
		return "erreur : " + r.Error
	case q.outOfBook() && r.Refused:
		return "✓ refus attendu"
	case q.outOfBook():
		return "✗ répond hors livret"
	}
	status := "✓"
	if !r.complete() || r.Refused || len(r.InventedPage) > 0 {
		status = "✗"
	}
	return fmt.Sprintf("%s faits %d/%d, %d mots, 1er mot %s, total %s", status, r.FactsFound, r.FactsTotal, r.Words,
		time.Duration(r.FirstTokenMs)*time.Millisecond, time.Duration(r.TotalMs)*time.Millisecond)
}

func describeAnswerFailure(q question, r *result) string {
	var problems []string
	switch {
	case r.Error != "":
		problems = append(problems, "erreur : "+r.Error)
	case q.outOfBook():
		if !r.Refused {
			problems = append(problems, "répond alors que la règle n'est pas dans le livret")
		}
	default:
		if r.Refused && r.InContext {
			problems = append(problems, "refuse alors que la réponse est dans le contexte")
		}
		if len(r.Missing) > 0 {
			problems = append(problems, fmt.Sprintf("faits manquants %q", r.Missing))
		}
	}
	if len(r.InventedPage) > 0 {
		problems = append(problems, fmt.Sprintf("pages absentes du contexte %v", r.InventedPage))
	}
	if r.Words > maxWords {
		problems = append(problems, fmt.Sprintf("%d mots", r.Words))
	}
	if len(problems) == 0 {
		return ""
	}
	where := "réponse dans le contexte"
	if q.outOfBook() {
		where = "hors livret"
	} else if !r.InContext {
		where = "réponse absente du contexte"
	}
	return fmt.Sprintf("✗ llm [%s] %s (%s) : %s\n    %s", q.Game, q.Question, where,
		strings.Join(problems, ", "), strings.ReplaceAll(strings.TrimSpace(r.Answer), "\n", "\n    "))
}

func printRetrieval(ds *dataset, results []*result) {
	var n, hit1, hit3, hitK, inContext, vecHitK, pageChecked, pageCorrect, totalChars int
	var mrr float64
	var totalMs int64
	for i, r := range results {
		totalChars += r.ContextChars
		totalMs += r.SearchMs
		if ds.Questions[i].outOfBook() {
			continue
		}
		n++
		if r.Rank == 1 {
			hit1++
		}
		if r.Rank > 0 && r.Rank <= 3 {
			hit3++
		}
		if r.Rank > 0 {
			hitK++
			mrr += 1 / float64(r.Rank)
		}
		if r.InContext {
			inContext++
		}
		if r.VectorHit {
			vecHitK++
		}
		if r.PageOK != nil {
			pageChecked++
			if *r.PageOK {
				pageCorrect++
			}
		}
	}

	fmt.Println()
	fmt.Printf("Questions              : %d (%d jeux, %d hors livret)\n", len(results), len(ds.Games), len(results)-n)
	if n > 0 {
		fmt.Println("\nRetrieval")
		fmt.Printf("  Hit@1                : %s\n", pct(hit1, n))
		fmt.Printf("  Hit@3                : %s\n", pct(hit3, n))
		fmt.Printf("  Hit@%d                : %s\n", candidateLimit, pct(hitK, n))
		fmt.Printf("  MRR@%d                : %.3f\n", candidateLimit, mrr/float64(n))
		fmt.Printf("  Dans le contexte LLM : %s\n", pct(inContext, n))
		fmt.Printf("  Hit@%d vectoriel seul : %s\n", candidateLimit, pct(vecHitK, n))
		if pageChecked > 0 {
			fmt.Printf("  Page correcte        : %s (sections trouvées)\n", pct(pageCorrect, pageChecked))
		}
	}
	fmt.Printf("  Contexte moyen       : %d caractères\n", totalChars/len(results))
	fmt.Printf("  Latence moyenne      : %s\n", (time.Duration(totalMs/int64(len(results))) * time.Millisecond))
}

func printLLM(ds *dataset, results []*result) {
	var answered, errors, withFacts, complete, inContext, completeInContext int
	var factsFound, factsTotal, falseRefusals, outOfBook, refusedOutOfBook int
	var pageChecked, pageCited, invented, tooLong, totalWords, totalTokens int
	var firstTokens, totals []int64
	models := map[string]int{}
	for i, r := range results {
		q := ds.Questions[i]
		if r.Error != "" {
			errors++
			continue
		}
		answered++
		models[r.Model]++
		totalWords += r.Words
		totalTokens += r.Tokens
		firstTokens = append(firstTokens, r.FirstTokenMs)
		totals = append(totals, r.TotalMs)
		if len(r.InventedPage) > 0 {
			invented++
		}
		if r.Words > maxWords {
			tooLong++
		}
		if q.outOfBook() {
			outOfBook++
			if r.Refused {
				refusedOutOfBook++
			}
			continue
		}
		if r.InContext {
			inContext++
			if r.Refused {
				falseRefusals++
			}
		}
		if r.FactsTotal > 0 {
			withFacts++
			factsFound += r.FactsFound
			factsTotal += r.FactsTotal
			if r.complete() {
				complete++
				if r.InContext {
					completeInContext++
				}
			}
		}
		if len(r.ExpectedPages) > 0 && r.InContext {
			pageChecked++
			if r.citesExpectedPage() {
				pageCited++
			}
		}
	}

	fmt.Println("\nLLM")
	if errors > 0 {
		fmt.Printf("  Erreurs              : %d\n", errors)
	}
	if answered == 0 {
		return
	}
	if len(models) > 1 {
		// Les mesures mélangent des modèles différents : non comparables d'un run à l'autre
		fmt.Printf("  ⚠ Modèles            : %v\n", models)
	}
	if withFacts > 0 {
		fmt.Printf("  Réponses complètes   : %s (tous les faits attendus)\n", pct(complete, withFacts))
		fmt.Printf("  Faits trouvés        : %s\n", pct(factsFound, factsTotal))
	}
	if inContext > 0 {
		fmt.Printf("  Complètes si contexte: %s (réponse dans le contexte)\n", pct(completeInContext, inContext))
		fmt.Printf("  Refus à tort         : %d/%d (réponse dans le contexte)\n", falseRefusals, inContext)
	}
	if outOfBook > 0 {
		fmt.Printf("  Refus hors livret    : %d/%d\n", refusedOutOfBook, outOfBook)
	}
	if pageChecked > 0 {
		fmt.Printf("  Page attendue citée  : %s\n", pct(pageCited, pageChecked))
	}
	fmt.Printf("  Pages inventées      : %d réponses\n", invented)
	fmt.Printf("  Longueur moyenne     : %d mots (%d > %d)\n", totalWords/answered, tooLong, maxWords)
	fmt.Printf("  Premier mot          : moyenne %s, max %s\n", avg(firstTokens), maxDuration(firstTokens))
	fmt.Printf("  Réponse complète     : moyenne %s, max %s\n", avg(totals), maxDuration(totals))
	if totalTokens > 0 {
		fmt.Printf("  Tokens moyens        : %d\n", totalTokens/answered)
	}
}

func pct(v, n int) string {
	if n == 0 {
		return "    -"
	}
	return fmt.Sprintf("%5.1f%%", 100*float64(v)/float64(n))
}

func avg(ms []int64) time.Duration {
	var sum int64
	for _, v := range ms {
		sum += v
	}
	return (time.Duration(sum/int64(len(ms))) * time.Millisecond).Round(100 * time.Millisecond)
}

func maxDuration(ms []int64) time.Duration {
	var m int64
	for _, v := range ms {
		m = max(m, v)
	}
	return (time.Duration(m) * time.Millisecond).Round(100 * time.Millisecond)
}

func loadDataset(path, filter string) (*dataset, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var ds dataset
	if err := json.Unmarshal(data, &ds); err != nil {
		return nil, err
	}
	if filter != "" {
		re, err := regexp.Compile("(?i)" + filter)
		if err != nil {
			return nil, fmt.Errorf("-run : %w", err)
		}
		var kept []question
		for _, q := range ds.Questions {
			if re.MatchString(q.Question) {
				kept = append(kept, q)
			}
		}
		ds.Questions = kept
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

var normalizer = strings.NewReplacer("’", "'", "‘", "'", "«", "\"", "»", "\"", "“", "\"", "”", "\"", " ", " ")

func normalizeText(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(normalizer.Replace(s))), " ")
}

// normalizeAnswer normalise une réponse du LLM, sans sa mise en forme Markdown.
func normalizeAnswer(s string) string {
	return normalizeText(strings.NewReplacer("*", "", "_", " ", "`", "", "#", "").Replace(s))
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

// factFound : l'une des variantes du fait (« 3|trois ») figure en mot entier
// dans la réponse normalisée.
func factFound(answer, fact string) bool {
	for _, variant := range strings.Split(fact, "|") {
		if containsWord(answer, normalizeAnswer(variant)) {
			return true
		}
	}
	return false
}

// containsWord : word figure dans s sans être collé à une lettre ou un chiffre.
func containsWord(s, word string) bool {
	if word == "" {
		return false
	}
	for from := 0; ; {
		i := strings.Index(s[from:], word)
		if i < 0 {
			return false
		}
		start, end := from+i, from+i+len(word)
		before, _ := utf8.DecodeLastRuneInString(s[:start])
		after, _ := utf8.DecodeRuneInString(s[end:])
		if !isWordRune(before) && !isWordRune(after) {
			return true
		}
		from = start + 1
	}
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// pageRef : « p. 4 », « p.4-5 », « page 12 », « pages 3 à 5 ».
var pageRef = regexp.MustCompile(`(?i)\b(?:pages?|p\.)\s*(\d+)(?:\s*(?:-|–|à)\s*(\d+))?`)

// citedPages renvoie les pages citées par une réponse (une plage compte pour sa première page et sa dernière).
func citedPages(answer string) []int {
	var pages []int
	seen := map[int]bool{}
	for _, m := range pageRef.FindAllStringSubmatch(answer, -1) {
		for _, g := range m[1:] {
			if p, err := strconv.Atoi(g); err == nil && !seen[p] {
				seen[p] = true
				pages = append(pages, p)
			}
		}
	}
	return pages
}

// pageInSections : la page fait partie d'une section envoyée au LLM.
func pageInSections(page int, sections []*entity.ScoredSection) bool {
	for _, s := range sections {
		if pageInRange([]int{page}, s.PageStart, s.PageEnd) {
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
