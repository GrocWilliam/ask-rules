// application/usecase/reprocess_game_test.go — Tests du remplacement des sections au reprocess
package usecase_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"ask-rules-server/internal/application/usecase"
	"ask-rules-server/internal/domain/entity"
)

func reprocessFixture(process func(sections *mockSectionRepo) error) (*usecase.ReprocessGameUseCase, *mockGameRepo, *mockSectionRepo) {
	game := &entity.Game{ID: "catan", Name: "Catan", Stats: map[string]interface{}{"files": []interface{}{"catan/regles.pdf"}}}
	gameRepo := &mockGameRepo{
		findByIDFunc: func(ctx context.Context, id string) (*entity.Game, error) { return game, nil },
	}
	sectionRepo := &mockSectionRepo{ids: []string{"old-1", "old-2"}}
	pipeline := &mockPipeline{processFunc: func(ctx context.Context, opts *usecase.ImportOptions) error {
		return process(sectionRepo)
	}}
	return usecase.NewReprocessGameUseCase(gameRepo, sectionRepo, pipeline, nil), gameRepo, sectionRepo
}

// Un pipeline réussi remplace les anciennes sections et invalide le cache du jeu.
func TestReprocess_ReplacesSectionsOnSuccess(t *testing.T) {
	uc, gameRepo, sectionRepo := reprocessFixture(func(sections *mockSectionRepo) error {
		sections.ids = append(sections.ids, "new-1")
		return nil
	})

	if err := uc.Execute(context.Background(), &usecase.ReprocessRequest{GameID: "catan"}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !slices.Equal(sectionRepo.ids, []string{"new-1"}) {
		t.Errorf("sections = %v, attendu [new-1]", sectionRepo.ids)
	}
	if !slices.Equal(gameRepo.touched, []string{"catan"}) {
		t.Errorf("touched = %v, attendu [catan]", gameRepo.touched)
	}
}

// Un pipeline en échec garde les anciennes sections et retire celles déjà insérées.
func TestReprocess_KeepsSectionsOnFailure(t *testing.T) {
	uc, gameRepo, sectionRepo := reprocessFixture(func(sections *mockSectionRepo) error {
		sections.ids = append(sections.ids, "partial-1")
		return errors.New("pdftotext: exit status 1")
	})

	if err := uc.Execute(context.Background(), &usecase.ReprocessRequest{GameID: "catan"}); err == nil {
		t.Fatal("Execute: erreur attendue")
	}
	if !slices.Equal(sectionRepo.ids, []string{"old-1", "old-2"}) {
		t.Errorf("sections = %v, attendu [old-1 old-2]", sectionRepo.ids)
	}
	if len(gameRepo.touched) != 0 {
		t.Errorf("touched = %v, le cache ne doit pas être invalidé", gameRepo.touched)
	}
}
