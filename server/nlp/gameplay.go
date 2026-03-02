// nlp/gameplay.go — Extraction structurée des éléments de gameplay
//
// Analyse l'ensemble des chunks d'un jeu et extrait :
//   - Mise en place (setup)
//   - Structure des manches / rounds
//   - Déroulement d'un tour
//   - Fin de partie / conditions de victoire
//   - Mécaniques de jeu avec leur contexte textuel
//   - Phases / étapes ordonnées du déroulement
package nlp

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// ── Structures ────────────────────────────────────────────────────────────────

// GameplayData contient l'ensemble des éléments de gameplay extraits des règles.
type GameplayData struct {
	Setup     *GameplaySection `json:"setup,omitempty"`
	Rounds    *GameplaySection `json:"rounds,omitempty"`
	Turns     *GameplaySection `json:"turns,omitempty"`
	EndGame   *GameplaySection `json:"end_game,omitempty"`
	Mechanics []MechanicDetail `json:"mechanics,omitempty"`
	Phases    []GamePhase      `json:"phases,omitempty"`
}

// GameplaySection représente une section structurée du gameplay.
type GameplaySection struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Keywords    []string `json:"keywords,omitempty"`
}

// MechanicDetail représente une mécanique de jeu avec son contexte.
type MechanicDetail struct {
	Type        string `json:"type"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// GamePhase représente une phase / étape ordonnée du déroulement d'une partie.
type GamePhase struct {
	Name  string   `json:"name"`
	Order int      `json:"order"`
	Steps []string `json:"steps,omitempty"`
}

// ── Labels lisibles des mécaniques ───────────────────────────────────────────

var mechanicLabels = map[Mechanic]string{
	MechDiceRolling:     "Lancer de dés",
	MechCardDrafting:    "Sélection de cartes",
	MechWorkerPlacement: "Placement d'ouvriers",
	MechAreaControl:     "Contrôle de zone",
	MechDeckBuilding:    "Construction de deck",
	MechResourceManage:  "Gestion des ressources",
	MechTilePlacement:   "Placement de tuiles",
	MechAuction:         "Enchères",
	MechCoopPlay:        "Jeu coopératif",
	MechTrading:         "Commerce / Échange",
	MechRouteBuilding:   "Construction de routes",
	MechPushYourLuck:    "Stop ou encore",
	MechSetCollection:   "Collection de sets",
	MechHandMgmt:        "Gestion de la main",
	MechActionPoints:    "Points d'action",
	MechVariablePhase:   "Ordre de jeu variable",
	MechElimination:     "Élimination de joueur",
	MechMemory:          "Mémoire",
	MechBluffing:        "Bluff",
	MechRoleAssign:      "Attribution de rôles",
	MechCRPG:            "Progression de personnage",
	MechDrafting:        "Draft",
	MechSimultaneous:    "Actions simultanées",
	MechProgression:     "Progression",
	MechModularBoard:    "Plateau modulable",
	MechEventCards:      "Cartes événements",
}

// ── Expressions régulières ────────────────────────────────────────────────────

var (
	// Détection de mentions de manches/rounds
	roundRe = regexp.MustCompile(`(?i)(manche|round|rotation|cycle de jeu|tour de table)`)

	// Liste numérotée : "1. texte" ou "1) texte"
	numberedItemRe = regexp.MustCompile(`(?m)^\s*(\d+)[\.\)]\s+(.{5,200})`)

	// Séparateur de phrases
	sentenceEndRe = regexp.MustCompile(`[.!?]+[\s\n]+`)
)

// ── Points d'entrée publics ───────────────────────────────────────────────────

// ExtractGameplay analyse l'ensemble des chunks d'un jeu et retourne la
// structure de gameplay complète : mise en place, tours, manches, fin de partie,
// mécaniques identifiées avec contexte, et phases/étapes ordonnées.
func ExtractGameplay(chunks []string) *GameplayData {
	var setupTexts, roundTexts, turnTexts, endTexts []string

	for _, text := range chunks {
		stype := DetectSectionType("", text)
		lowerNorm := strings.ToLower(normalize(text))

		switch stype {
		case TypeSetup:
			setupTexts = append(setupTexts, text)
		case TypeTurn:
			turnTexts = append(turnTexts, text)
		case TypeEnd:
			endTexts = append(endTexts, text)
		}
		// Les manches peuvent être mentionnées dans tout type de section
		if roundRe.MatchString(lowerNorm) && stype != TypeSetup {
			roundTexts = append(roundTexts, text)
		}
	}

	data := &GameplayData{}
	if len(setupTexts) > 0 {
		data.Setup = buildSection("Mise en place", setupTexts)
	}
	if len(roundTexts) > 0 {
		data.Rounds = buildSection("Structure des manches", roundTexts)
	}
	if len(turnTexts) > 0 {
		data.Turns = buildSection("Déroulement d'un tour", turnTexts)
	}
	if len(endTexts) > 0 {
		data.EndGame = buildSection("Fin de partie", endTexts)
	}

	data.Mechanics = extractMechanicsWithContext(chunks)
	data.Phases = extractPhases(chunks)

	return data
}

// IsEmpty retourne true si le GameplayData ne contient aucune information utile.
func (g *GameplayData) IsEmpty() bool {
	return g.Setup == nil && g.Rounds == nil && g.Turns == nil &&
		g.EndGame == nil && len(g.Mechanics) == 0 && len(g.Phases) == 0
}

// ── Extraction interne ───────────────────────────────────────────────────────

// buildSection construit une GameplaySection depuis un ensemble de textes bruts.
// Sélectionne les phrases les plus représentatives et les mots-clés du domaine.
func buildSection(title string, texts []string) *GameplaySection {
	const maxDescLen = 700 // caractères max de la description

	var descParts []string
	for i, t := range texts {
		if i >= 3 {
			break
		}
		sentences := splitSentences(t)
		var part strings.Builder
		for _, s := range sentences {
			if part.Len()+len(s)+2 > maxDescLen/2 {
				break
			}
			if part.Len() > 0 {
				part.WriteString(" ")
			}
			part.WriteString(strings.TrimSpace(s))
			part.WriteString(".")
		}
		if part.Len() > 0 {
			descParts = append(descParts, strings.TrimSpace(part.String()))
		}
	}

	desc := strings.Join(descParts, "\n\n")
	if runes := []rune(desc); len(runes) > maxDescLen {
		desc = string(runes[:maxDescLen]) + "…"
	}

	// Mots-clés depuis tous les textes (max 12)
	kws := ExtractKeywords(strings.Join(texts, " "))
	if len(kws) > 12 {
		kws = kws[:12]
	}

	return &GameplaySection{
		Title:       title,
		Description: desc,
		Keywords:    kws,
	}
}

// extractMechanicsWithContext trouve les mécaniques présentes et associe à chacune
// la première phrase du texte qui la mentionne explicitement.
func extractMechanicsWithContext(chunks []string) []MechanicDetail {
	mechDesc := map[Mechanic]string{}

	for _, text := range chunks {
		lowerNorm := strings.ToLower(normalize(text))
		sentences := splitSentences(text)

		for mech, keywords := range mechanicKeywords {
			if _, already := mechDesc[mech]; already {
				continue
			}
			// Vérifier la présence dans le texte entier
			present := false
			for _, kw := range keywords {
				if strings.Contains(lowerNorm, strings.ToLower(normalize(kw))) {
					present = true
					break
				}
			}
			if !present {
				continue
			}
			// Trouver la première phrase contenant le mot-clé
			for _, sent := range sentences {
				sentLow := strings.ToLower(normalize(sent))
				for _, kw := range keywords {
					if strings.Contains(sentLow, strings.ToLower(normalize(kw))) {
						s := strings.TrimSpace(sent)
						if len([]rune(s)) > 200 {
							s = string([]rune(s)[:200]) + "…"
						}
						mechDesc[mech] = s
						break
					}
				}
				if _, ok := mechDesc[mech]; ok {
					break
				}
			}
		}
	}

	var result []MechanicDetail
	for mech, desc := range mechDesc {
		label := mech
		if l, ok := mechanicLabels[mech]; ok {
			label = l
		}
		result = append(result, MechanicDetail{
			Type:        mech,
			Label:       label,
			Description: desc,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Label < result[j].Label
	})
	return result
}

// extractPhases détecte les phases/étapes ordonnées du déroulement d'une partie.
// Cherche les blocs de texte contenant des listes numérotées (au moins 2 éléments).
// La séquence avec le plus d'éléments est retenue comme structure de référence.
func extractPhases(chunks []string) []GamePhase {
	type candidate struct {
		phases []GamePhase
		count  int
	}
	var best candidate

	for _, text := range chunks {
		// Ne chercher que dans les sections de type tour, setup ou général
		stype := DetectSectionType("", text)
		if stype == TypeEnd || stype == TypeScoring || stype == TypeComponent {
			continue
		}

		matches := numberedItemRe.FindAllStringSubmatch(text, -1)
		if len(matches) < 2 {
			continue
		}

		var phases []GamePhase
		seen := map[int]bool{}
		for _, m := range matches {
			if len(m) < 3 {
				continue
			}
			var order int
			fmt.Sscanf(m[1], "%d", &order)
			if order == 0 || seen[order] {
				continue
			}
			seen[order] = true

			name := strings.TrimSpace(m[2])
			// Nettoyer les artefacts de fin de ligne
			if idx := strings.IndexAny(name, "\n\r"); idx >= 0 {
				name = name[:idx]
			}
			if len([]rune(name)) > 90 {
				name = string([]rune(name)[:90]) + "…"
			}
			phases = append(phases, GamePhase{
				Name:  name,
				Order: order,
			})
		}

		// Trier par ordre dans ce bloc
		sort.Slice(phases, func(i, j int) bool {
			return phases[i].Order < phases[j].Order
		})

		// N'enrichir avec sous-étapes que pour la séquence la plus longue trouvée
		if len(phases) > best.count {
			// Chercher des sous-étapes pour chaque phase (lignes suivant l'item)
			for i := range phases {
				phases[i].Steps = extractStepsForPhase(text, phases[i].Order, phases)
			}
			best = candidate{phases: phases, count: len(phases)}
		}
	}

	result := best.phases
	if len(result) > 15 {
		result = result[:15]
	}
	return result
}

// extractStepsForPhase extrait les sous-éléments (lignes avec tiret, •, –) qui
// suivent immédiatement un item numéroté dans le texte.
func extractStepsForPhase(text string, order int, allPhases []GamePhase) []string {
	// Trouver la position de l'item n°order dans le texte
	startRe := regexp.MustCompile(fmt.Sprintf(`(?m)^\s*%d[\.\)]\s+`, order))
	loc := startRe.FindStringIndex(text)
	if loc == nil {
		return nil
	}

	// Trouver la fin : début de l'item suivant ou fin de texte
	endIdx := len(text)
	nextOrder := order + 1
	for _, p := range allPhases {
		if p.Order == nextOrder {
			nextRe := regexp.MustCompile(fmt.Sprintf(`(?m)^\s*%d[\.\)]\s+`, nextOrder))
			if loc2 := nextRe.FindStringIndex(text[loc[0]:]); loc2 != nil {
				endIdx = loc[0] + loc2[0]
			}
			break
		}
	}

	block := text[loc[0]:endIdx]
	// Chercher les sous-éléments : lignes commençant par tiret, •, *, –
	subRe := regexp.MustCompile(`(?m)^\s*[-•\*–]\s+(.{3,100})`)
	var steps []string
	for _, m := range subRe.FindAllStringSubmatch(block, -1) {
		if len(m) >= 2 {
			steps = append(steps, strings.TrimSpace(m[1]))
		}
	}
	return steps
}

// ── Utilitaires ──────────────────────────────────────────────────────────────

// splitSentences découpe un texte en phrases (séparateurs . ! ?).
func splitSentences(text string) []string {
	parts := sentenceEndRe.Split(text, -1)
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if len([]rune(p)) >= 8 {
			result = append(result, p)
		}
	}
	return result
}
