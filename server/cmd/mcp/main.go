// cmd/mcp/main.go — Serveur MCP (stdio) : accès à la base et au LLM déployés
//
// Donne à un assistant (Claude Code…) des outils pour inspecter la base
// PostgreSQL en lecture seule et interroger le LLM configuré par LLM_*.
//
// Usage (depuis server/) :
//
//	go run ./cmd/mcp
//
// Configuration : le même .env que le serveur. MCP_DATABASE_URL, si défini,
// remplace DATABASE_URL (pour un rôle PostgreSQL en lecture seule).
// Les migrations ne sont jamais lancées.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	"ask-rules-server/internal/config"
	"ask-rules-server/internal/llm"
	"ask-rules-server/internal/store"

	"github.com/jackc/pgx/v5"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	// queryTimeout : durée max d'une requête SQL (statement_timeout)
	queryTimeout = 15 * time.Second
	// defaultRowLimit / maxRowLimit : lignes renvoyées par db_query
	defaultRowLimit = 100
	maxRowLimit     = 1000
	// maxValueLen : longueur max d'une valeur texte (les embeddings font ~8 Ko)
	maxValueLen = 1000
	// defaultMaxTokens : longueur max d'une réponse de llm_chat
	defaultMaxTokens = 400
)

func main() {
	// stdout est réservé au protocole MCP : les logs vont sur stderr
	log.SetOutput(os.Stderr)

	if err := config.Load(); err != nil {
		log.Fatalf("config : %v", err)
	}
	if url := os.Getenv("MCP_DATABASE_URL"); url != "" {
		config.C.DatabaseURL = url
	}
	if err := store.Connect(); err != nil {
		log.Fatalf("base : %v", err)
	}
	defer store.Close()

	llmClient := llm.New()

	server := mcp.NewServer(&mcp.Implementation{Name: "ask-rules", Version: "1.0.0"}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "db_schema",
		Description: "Liste les tables du schéma public de la base déployée, avec leurs colonnes et leur nombre approximatif de lignes.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, dbSchema)

	mcp.AddTool(server, &mcp.Tool{
		Name: "db_query",
		Description: "Exécute une requête SQL (une seule instruction) sur la base déployée, dans une transaction en lecture seule " +
			"avec un délai de 15 s. Les textes longs (embeddings…) sont tronqués : sélectionnez les colonnes utiles.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, dbQuery)

	mcp.AddTool(server, &mcp.Tool{
		Name: "llm_chat",
		Description: "Envoie des messages bruts au LLM déployé (API chat/completions), sans le prompt de l'arbitre. " +
			"Réveille le LLM s'il est en veille (peut prendre plusieurs minutes).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in llmChatInput) (*mcp.CallToolResult, *llm.Response, error) {
		return llmChat(ctx, llmClient, in)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "llm_answer_rule",
		Description: "Pose une question au LLM déployé exactement comme /api/ask : prompt système de l'arbitre, " +
			"extraits du livret fournis en contexte (ex. le contenu de sections lues avec db_query). Sans recherche ni cache.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in llmAnswerInput) (*mcp.CallToolResult, *llm.Response, error) {
		resp, err := llmClient.Query(ctx, in.Question, in.Context, in.History, nil)
		return nil, resp, err
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		log.Fatalf("serveur MCP : %v", err)
	}
}

// ── Base de données ─────────────────────────────────────

type dbSchemaOutput struct {
	Tables []tableInfo `json:"tables"`
}

type tableInfo struct {
	Name          string       `json:"name"`
	EstimatedRows int64        `json:"estimated_rows"`
	Columns       []columnInfo `json:"columns"`
}

type columnInfo struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
}

func dbSchema(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, dbSchemaOutput, error) {
	rows, err := store.Pool.Query(ctx, `
		SELECT c.table_name, c.column_name, c.udt_name, c.is_nullable = 'YES',
		       COALESCE(s.n_live_tup, 0)
		FROM information_schema.columns c
		LEFT JOIN pg_stat_user_tables s ON s.relname = c.table_name AND s.schemaname = c.table_schema
		WHERE c.table_schema = 'public'
		ORDER BY c.table_name, c.ordinal_position`)
	if err != nil {
		return nil, dbSchemaOutput{}, err
	}
	defer rows.Close()

	var out dbSchemaOutput
	for rows.Next() {
		var table string
		var col columnInfo
		var estimated int64
		if err := rows.Scan(&table, &col.Name, &col.Type, &col.Nullable, &estimated); err != nil {
			return nil, dbSchemaOutput{}, err
		}
		if n := len(out.Tables); n == 0 || out.Tables[n-1].Name != table {
			out.Tables = append(out.Tables, tableInfo{Name: table, EstimatedRows: estimated})
		}
		last := &out.Tables[len(out.Tables)-1]
		last.Columns = append(last.Columns, col)
	}
	return nil, out, rows.Err()
}

type dbQueryInput struct {
	SQL   string `json:"sql" jsonschema:"requête SQL, une seule instruction (SELECT, EXPLAIN…)"`
	Limit int    `json:"limit,omitempty" jsonschema:"lignes max renvoyées (défaut 100, max 1000)"`
}

type dbQueryOutput struct {
	Columns   []string `json:"columns"`
	Rows      [][]any  `json:"rows"`
	Truncated bool     `json:"truncated" jsonschema:"vrai si la requête renvoie plus de lignes que limit"`
}

// dbQuery exécute la requête dans une transaction en lecture seule, toujours
// annulée. pgx prépare la requête : une seule instruction est acceptée, on ne
// peut donc pas glisser un COMMIT suivi d'une écriture.
func dbQuery(ctx context.Context, _ *mcp.CallToolRequest, in dbQueryInput) (*mcp.CallToolResult, dbQueryOutput, error) {
	limit := in.Limit
	if limit <= 0 {
		limit = defaultRowLimit
	}
	limit = min(limit, maxRowLimit)

	tx, err := store.Pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, dbQueryOutput{}, err
	}
	defer tx.Rollback(context.Background())

	// Un SELECT (et non un SET) : il prend le snapshot de la transaction, après
	// quoi PostgreSQL refuse de la repasser en lecture-écriture.
	if _, err := tx.Exec(ctx, "SELECT set_config('statement_timeout', $1, true)", fmt.Sprint(queryTimeout.Milliseconds())); err != nil {
		return nil, dbQueryOutput{}, err
	}

	rows, err := tx.Query(ctx, in.SQL)
	if err != nil {
		return nil, dbQueryOutput{}, err
	}
	defer rows.Close()

	out := dbQueryOutput{Rows: [][]any{}}
	for _, field := range rows.FieldDescriptions() {
		out.Columns = append(out.Columns, field.Name)
	}
	for rows.Next() {
		if len(out.Rows) == limit {
			out.Truncated = true
			break
		}
		values, err := rows.Values()
		if err != nil {
			return nil, dbQueryOutput{}, err
		}
		for i, v := range values {
			values[i] = jsonValue(v)
		}
		out.Rows = append(out.Rows, values)
	}
	return nil, out, rows.Err()
}

// jsonValue rend une valeur lisible en JSON : UUID en texte, textes longs tronqués.
func jsonValue(v any) any {
	switch val := v.(type) {
	case [16]byte:
		return fmt.Sprintf("%x-%x-%x-%x-%x", val[0:4], val[4:6], val[6:8], val[8:10], val[10:16])
	case []byte:
		return truncate(string(val))
	case string:
		return truncate(val)
	default:
		return v
	}
}

func truncate(s string) string {
	if len(s) <= maxValueLen {
		return s
	}
	cut := strings.ToValidUTF8(s[:maxValueLen], "")
	return fmt.Sprintf("%s… [%d octets au total]", cut, len(s))
}

// ── LLM ─────────────────────────────────────────────────

type llmChatInput struct {
	Messages    []llm.Message `json:"messages" jsonschema:"messages de la conversation (role : system, user ou assistant)"`
	MaxTokens   int           `json:"max_tokens,omitempty" jsonschema:"longueur max de la réponse (défaut 400)"`
	Temperature *float64      `json:"temperature,omitempty" jsonschema:"température (défaut 0.3)"`
}

func llmChat(ctx context.Context, client *llm.Client, in llmChatInput) (*mcp.CallToolResult, *llm.Response, error) {
	if len(in.Messages) == 0 {
		return nil, nil, fmt.Errorf("messages vide")
	}
	maxTokens := in.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}
	temperature := 0.3
	if in.Temperature != nil {
		temperature = *in.Temperature
	}
	resp, err := client.Chat(ctx, in.Messages, maxTokens, temperature)
	return nil, resp, err
}

type llmAnswerInput struct {
	Question string         `json:"question" jsonschema:"question du joueur"`
	Context  string         `json:"context" jsonschema:"extraits du livret envoyés au LLM, comme le contexte construit par /api/ask"`
	History  []llm.ChatTurn `json:"history,omitempty" jsonschema:"échanges précédents, pour une question de suivi"`
}
