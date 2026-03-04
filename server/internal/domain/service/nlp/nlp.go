// nlp/nlp.go — Traitement NLP français (détection mécaniques, intention)
package nlp

import (
	"regexp"
	"strings"
	"unicode"
)

// Mechanic représente une mécanique de jeu reconnue.
type Mechanic = string

const (
	MechDiceRolling     Mechanic = "dice_rolling"
	MechCardDrafting    Mechanic = "card_drafting"
	MechWorkerPlacement Mechanic = "worker_placement"
	MechAreaControl     Mechanic = "area_control"
	MechDeckBuilding    Mechanic = "deck_building"
	MechResourceManage  Mechanic = "resource_management"
	MechTilePlacement   Mechanic = "tile_placement"
	MechAuction         Mechanic = "auction_bidding"
	MechCoopPlay        Mechanic = "cooperative_play"
	MechTrading         Mechanic = "trading"
	MechRouteBuilding   Mechanic = "route_building"
	MechPushYourLuck    Mechanic = "push_your_luck"
	MechSetCollection   Mechanic = "set_collection"
	MechHandMgmt        Mechanic = "hand_management"
	MechActionPoints    Mechanic = "action_points"
	MechVariablePhase   Mechanic = "variable_phase_order"
	MechElimination     Mechanic = "player_elimination"
	MechStockholds      Mechanic = "stock_holding"
	MechMemory          Mechanic = "memory"
	MechBluffing        Mechanic = "bluffing"
	MechRoleAssign      Mechanic = "role_assignment"
	MechCRPG            Mechanic = "crpg_progression"
	MechDrafting        Mechanic = "drafting"
	MechSimultaneous    Mechanic = "simultaneous_action"
	MechProgression     Mechanic = "progression"
	MechModularBoard    Mechanic = "modular_board"
	MechEventCards      Mechanic = "event_cards"
)

var mechanicKeywords = map[Mechanic][]string{
	MechDiceRolling:     {"dé", "dés", "lancer", "lancé", "jet de dé", "résultat du dé"},
	MechCardDrafting:    {"draft", "sélection de carte", "choisir une carte", "piocher"},
	MechWorkerPlacement: {"ouvrier", "meeple", "placer son", "placer un travailleur"},
	MechAreaControl:     {"contrôle de zone", "territoire", "région", "domination"},
	MechDeckBuilding:    {"construction de deck", "améliorer son deck", "ajouter des cartes"},
	MechResourceManage:  {"ressource", "bois", "pierre", "or", "nourriture", "gérer"},
	MechTilePlacement:   {"tuile", "hexagone", "plateau modulable", "poser une tuile"},
	MechAuction:         {"enchère", "mise", "vente aux enchères", "offre"},
	MechCoopPlay:        {"coopératif", "ensemble", "équipe", "collectif"},
	MechTrading:         {"échange", "commerce", "vendre", "acheter", "négocier"},
	MechRouteBuilding:   {"route", "chemin de fer", "réseau", "relier", "connexion"},
	MechPushYourLuck:    {"relancer", "risque", "stop ou encore", "tenter sa chance"},
	MechSetCollection:   {"collection", "ensemble", "série", "jeu complet"},
	MechHandMgmt:        {"main", "cartes en main", "défausser", "conserver"},
	MechActionPoints:    {"point d'action", "PA", "actions disponibles", "dépenser"},
	MechVariablePhase:   {"ordre variable", "initiative", "priorité", "tour de jeu"},
	MechElimination:     {"élimination", "éliminé", "sortir du jeu"},
	MechMemory:          {"mémoire", "retourner", "face cachée", "mémoriser"},
	MechBluffing:        {"bluff", "mentir", "secret", "rôle caché"},
	MechRoleAssign:      {"rôle", "personnage", "faction", "classe"},
	MechCRPG:            {"expérience", "niveau", "progression", "compétence", "capacité"},
	MechDrafting:        {"draft", "rédiger", "sélection"},
	MechSimultaneous:    {"simultané", "en même temps", "révéler en même temps"},
	MechProgression:     {"progression", "avancer", "franchir", "étape"},
	MechModularBoard:    {"modulable", "aléatoire", "configuration variable"},
	MechEventCards:      {"événement", "carte événement", "effet immédiat"},
}

// SectionType représente le type d'une section de règles.
type SectionType = string

const (
	TypeSetup     SectionType = "setup"
	TypeTurn      SectionType = "turn"
	TypeScoring   SectionType = "scoring"
	TypeEnd       SectionType = "end"
	TypeComponent SectionType = "component"
	TypeSpecial   SectionType = "special"
	TypeExample   SectionType = "example"
	TypeGeneral   SectionType = "general"
)

var sectionTypePatterns = map[SectionType]*regexp.Regexp{
	TypeSetup:     regexp.MustCompile(`(?i)(mise en place|préparation|setup|début de partie|installation)`),
	TypeTurn:      regexp.MustCompile(`(?i)(tour de jeu|phase de jeu|déroulement|action du joueur)`),
	TypeScoring:   regexp.MustCompile(`(?i)(score|point|comptage|victoire|décompte)`),
	TypeEnd:       regexp.MustCompile(`(?i)(fin de partie|condition de victoire|gagner|perdre|remporter)`),
	TypeComponent: regexp.MustCompile(`(?i)(composant|matériel|contenu|carte|tuile|jeton|meeple|plateau|dé)`),
	TypeSpecial:   regexp.MustCompile(`(?i)(règle spéciale|exception|cas particulier|variante)`),
	TypeExample:   regexp.MustCompile(`(?i)(exemple|illustration|par exemple|e\.g\.)`),
}

// DetectMechanics détecte les mécaniques de jeu dans un texte.
func DetectMechanics(text string) []Mechanic {
	lower := strings.ToLower(normalize(text))
	found := []Mechanic{}
	seen := map[Mechanic]bool{}
	for mech, keywords := range mechanicKeywords {
		for _, kw := range keywords {
			if strings.Contains(lower, kw) {
				if !seen[mech] {
					found = append(found, mech)
					seen[mech] = true
				}
				break
			}
		}
	}
	return found
}

// DetectSectionType détermine le type d'une section à partir de son titre/contenu.
func DetectSectionType(title, text string) SectionType {
	combined := strings.ToLower(title + " " + text[:min(200, len(text))])
	for stype, re := range sectionTypePatterns {
		if re.MatchString(combined) {
			return stype
		}
	}
	return TypeGeneral
}

// IntentType catégorise l'intention d'une question.
type IntentType = string

const (
	IntentRules     IntentType = "rules"
	IntentSetup     IntentType = "setup"
	IntentScoring   IntentType = "scoring"
	IntentStrategy  IntentType = "strategy"
	IntentComponent IntentType = "component"
	IntentGeneral   IntentType = "general"
)

var intentPatterns = map[IntentType]*regexp.Regexp{
	IntentSetup:     regexp.MustCompile(`(?i)(comment (on |)commence|mise en place|début|préparer|setup|installation)`),
	IntentScoring:   regexp.MustCompile(`(?i)(points?|score|gagner|victoire|compter|décompte)`),
	IntentStrategy:  regexp.MustCompile(`(?i)(stratégie|conseil|meilleur|comment jouer|optimiser)`),
	IntentComponent: regexp.MustCompile(`(?i)(combien|composant|dé|carte|tuile|jeton|matériel|contenu)`),
}

// DetectIntent retourne l'intention d'une question.
func DetectIntent(question string) IntentType {
	lower := strings.ToLower(question)
	for intent, re := range intentPatterns {
		if re.MatchString(lower) {
			return intent
		}
	}
	return IntentRules
}

// ExtractKeywords extrait les mots-clés significatifs d'un texte.
// Les noms du domaine jeu (gameNouns) sont toujours inclus.
// Les stopwords et mots < 3 car. sont exclus.
func ExtractKeywords(text string) []string {
	words := strings.FieldsFunc(normalize(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	seen := map[string]bool{}
	var result []string
	for _, w := range words {
		lw := strings.ToLower(w)
		if seen[lw] {
			continue
		}
		seen[lw] = true
		// Toujours inclure les noms du domaine jeu
		if gameNouns[lw] {
			result = append(result, lw)
			continue
		}
		if len(lw) >= 3 && !frenchStopwords[lw] {
			result = append(result, lw)
		}
	}
	return result
}

// actionVerbs liste les verbes d'action typiques dans les règles de jeu.
var actionVerbs = []string{
	// Composants
	"piocher", "placer", "poser", "retirer", "défausser", "mélanger", "retourner",
	"révéler", "dévoiler", "distribuer", "récupérer", "prendre", "remettre",
	// Mouvement
	"déplacer", "avancer", "reculer", "traverser", "entrer", "sortir", "franchir",
	// Combat
	"attaquer", "défendre", "combattre", "éliminer", "capturer", "protéger",
	// Ressources
	"collecter", "récolter", "produire", "payer", "acheter", "vendre", "dépenser",
	"gagner", "perdre", "obtenir", "recevoir",
	// Construction
	"construire", "améliorer", "upgrader", "recruter", "déployer",
	// Cartes / main
	"jouer", "activer", "choisir", "sélectionner", "passer", "échanger",
	"lancer", "résoudre", "appliquer", "déclencher",
	// Tour
	"commencer", "terminer", "finir", "passer", "sauter", "reporter",
}

// actionVerbsNorm est la version normalisée (sans accents) des verbes.
var actionVerbsNorm []string

func init() {
	for _, v := range actionVerbs {
		actionVerbsNorm = append(actionVerbsNorm, normalize(v))
	}
}

// ExtractEntities extrait les entités nommées d'un texte :
// composants du jeu (gameNouns) et mots capitalisés (noms propres de cartes/zones).
func ExtractEntities(text string) []string {
	seen := map[string]bool{}
	var result []string

	// 1. Noms du domaine jeu présents dans le texte (normalisés)
	normText := normalize(text)
	wordsNorm := strings.FieldsFunc(normText, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	for _, w := range wordsNorm {
		lw := strings.ToLower(w)
		if gameNouns[lw] && !seen[lw] {
			seen[lw] = true
			result = append(result, lw)
		}
	}

	// 2. Mots capitalisés (noms propres hors début de phrase)
	sentences := regexp.MustCompile(`[.!?]\s+`).Split(text, -1)
	for _, sent := range sentences {
		words := strings.Fields(sent)
		for i, w := range words {
			if i == 0 {
				continue // ignorer le premier mot de la phrase
			}
			clean := strings.TrimFunc(w, func(r rune) bool { return !unicode.IsLetter(r) })
			if len(clean) >= 2 && unicode.IsUpper([]rune(clean)[0]) {
				lw := strings.ToLower(normalize(clean))
				if !frenchStopwords[lw] && !seen[lw] {
					seen[lw] = true
					result = append(result, clean)
				}
			}
		}
	}

	if len(result) > 20 {
		result = result[:20]
	}
	return result
}

// ExtractActions extrait les verbes d'action présents dans un texte.
func ExtractActions(text string) []string {
	lower := normalize(strings.ToLower(text))
	seen := map[string]bool{}
	var result []string
	for i, verbNorm := range actionVerbsNorm {
		if strings.Contains(lower, verbNorm) && !seen[verbNorm] {
			seen[verbNorm] = true
			result = append(result, actionVerbs[i])
		}
	}
	return result
}

// GameMeta contient les métadonnées extraites des règles du jeu.
type GameMeta struct {
	PlayersMin  int    `json:"players_min,omitempty"`
	PlayersMax  int    `json:"players_max,omitempty"`
	DurationMin int    `json:"duration_min,omitempty"`
	DurationMax int    `json:"duration_max,omitempty"`
	AgeMin      int    `json:"age_min,omitempty"`
	Language    string `json:"language,omitempty"`
}

var (
	rePlayersRange  = regexp.MustCompile(`(?i)(\d)\s*(?:à|[-–])\s*(\d)\s*joueurs?`)
	rePlayersSingle = regexp.MustCompile(`(?i)(\d+)\s*joueurs?`)
	reDurRange      = regexp.MustCompile(`(?i)(\d+)\s*(?:à|[-–])\s*(\d+)\s*min`)
	reDurSingle     = regexp.MustCompile(`(?i)(\d+)\s*min(?:utes?)?`)
	reAge           = regexp.MustCompile(`(?i)(?:à partir de |dès |age[: ]+)(\d+)\s*ans?`)
)

// ExtractGameMeta extrait les métadonnées de jeu (joueurs, durée, âge) depuis les règles.
// Typiquement appelé sur les premiers chunks (couverture / matériel).
func ExtractGameMeta(chunks []string) map[string]interface{} {
	meta := map[string]interface{}{}

	for _, chunk := range chunks {
		// Joueurs
		if _, ok := meta["players_min"]; !ok {
			if m := rePlayersRange.FindStringSubmatch(chunk); m != nil {
				meta["players_min"] = atoi(m[1])
				meta["players_max"] = atoi(m[2])
			} else if m := rePlayersSingle.FindStringSubmatch(chunk); m != nil {
				n := atoi(m[1])
				meta["players_min"] = n
				meta["players_max"] = n
			}
		}
		// Durée
		if _, ok := meta["duration_min"]; !ok {
			if m := reDurRange.FindStringSubmatch(chunk); m != nil {
				meta["duration_min"] = atoi(m[1])
				meta["duration_max"] = atoi(m[2])
			} else if m := reDurSingle.FindStringSubmatch(chunk); m != nil {
				d := atoi(m[1])
				meta["duration_min"] = d
				meta["duration_max"] = d
			}
		}
		// Âge
		if _, ok := meta["age_min"]; !ok {
			if m := reAge.FindStringSubmatch(chunk); m != nil {
				meta["age_min"] = atoi(m[1])
			}
		}
	}
	return meta
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n
}

// normalize enlève les accents pour la comparaison.
func normalize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case 'à', 'â', 'ä':
			b.WriteRune('a')
		case 'é', 'è', 'ê', 'ë':
			b.WriteRune('e')
		case 'î', 'ï':
			b.WriteRune('i')
		case 'ô', 'ö':
			b.WriteRune('o')
		case 'ù', 'û', 'ü':
			b.WriteRune('u')
		case 'ç':
			b.WriteRune('c')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ── Stopwords français (liste exhaustive) ─────────────────────────────────────

var frenchStopwords = map[string]bool{
	// Articles
	"le": true, "la": true, "les": true, "l": true, "un": true, "une": true,
	"des": true, "du": true, "de": true, "d": true,
	// Pronoms personnels
	"je": true, "tu": true, "il": true, "elle": true, "on": true, "nous": true,
	"vous": true, "ils": true, "elles": true, "me": true, "te": true, "se": true,
	"lui": true, "y": true, "en": true, "moi": true, "toi": true, "soi": true,
	// Pronoms démonstratifs
	"ce": true, "cet": true, "cette": true, "ces": true, "celui": true,
	"celle": true, "ceux": true, "celles": true, "cela": true, "ca": true, "ceci": true,
	// Déterminants possessifs
	"mon": true, "ma": true, "mes": true, "ton": true, "ta": true, "tes": true,
	"son": true, "sa": true, "ses": true, "notre": true, "votre": true, "vos": true,
	"nos": true, "leur": true, "leurs": true,
	// Prépositions
	"a": true, "au": true, "aux": true, "par": true, "pour": true, "sur": true,
	"sous": true, "dans": true, "avec": true, "sans": true, "entre": true,
	"vers": true, "chez": true, "contre": true, "malgre": true, "derriere": true,
	"devant": true, "autour": true, "parmi": true, "sauf": true, "hors": true,
	"depuis": true, "jusqu": true, "jusque": true, "environ": true,
	"selon": true, "afin": true, "lors": true,
	// Conjonctions
	"et": true, "ou": true, "mais": true, "donc": true, "or": true, "ni": true,
	"car": true, "que": true, "qu": true, "qui": true, "si": true, "comme": true,
	"quand": true, "lorsque": true, "puisque": true, "quoique": true, "tandis": true,
	"parce": true, "cependant": true, "neanmoins": true, "toutefois": true,
	"pourtant": true, "bien": true,
	// Adverbes courants
	"ne": true, "pas": true, "plus": true, "non": true, "oui": true, "jamais": true,
	"toujours": true, "souvent": true, "parfois": true, "rarement": true,
	"encore": true, "deja": true, "trop": true, "assez": true, "peu": true,
	"beaucoup": true, "tellement": true, "tres": true, "aussi": true, "alors": true,
	"ainsi": true, "moins": true, "seulement": true, "notamment": true,
	// Indéfinis
	"tout": true, "tous": true, "toute": true, "toutes": true, "chaque": true,
	"chacun": true, "chacune": true, "aucun": true, "aucune": true, "nul": true,
	"nulle": true, "autre": true, "autres": true, "meme": true, "memes": true,
	"certain": true, "certaine": true, "certains": true, "certaines": true,
	"quelque": true, "quelques": true, "plusieurs": true, "divers": true, "diverses": true,
	// Pronoms relatifs & interrogatifs
	"dont": true, "lequel": true, "laquelle": true, "lesquels": true,
	"lesquelles": true, "auquel": true, "duquel": true, "comment": true,
	"pourquoi": true, "quoi": true, "quel": true, "quelle": true, "quels": true,
	"quelles": true, "combien": true,
	// Être — toutes conjugaisons
	"etre": true, "ete": true, "etant": true, "suis": true, "es": true, "est": true,
	"sommes": true, "etes": true, "sont": true, "etais": true, "etait": true,
	"etions": true, "etiez": true, "etaient": true, "serai": true, "seras": true,
	"sera": true, "serons": true, "serez": true, "seront": true, "serais": true,
	"serait": true, "serions": true, "seriez": true, "seraient": true,
	"fus": true, "fut": true, "fumes": true, "futes": true, "furent": true,
	"soit": true, "soient": true, "soyons": true, "soyez": true,
	// Avoir — toutes conjugaisons
	"avoir": true, "eu": true, "ayant": true, "ai": true, "as": true,
	"avons": true, "avez": true, "ont": true, "avais": true, "avait": true,
	"avions": true, "aviez": true, "avaient": true, "aurai": true, "auras": true,
	"aura": true, "aurons": true, "aurez": true, "auront": true, "aurais": true,
	"aurait": true, "aurions": true, "auriez": true, "auraient": true,
	"eus": true, "eut": true, "eumes": true, "eutes": true, "eurent": true,
	"ait": true, "aient": true, "ayons": true, "ayez": true,
	// Faire
	"faire": true, "fait": true, "faite": true, "faits": true, "faites": true,
	"faisant": true, "fais": true, "faisons": true, "font": true, "faisais": true,
	"faisait": true, "faisions": true, "faisiez": true, "faisaient": true,
	"ferai": true, "feras": true, "fera": true, "ferons": true, "ferez": true,
	"feront": true, "ferais": true, "ferait": true, "ferions": true, "feriez": true,
	"feraient": true, "fis": true, "fit": true, "fimes": true, "fites": true,
	"firent": true, "fasse": true, "fassent": true, "fassions": true, "fassiez": true,
	// Aller
	"aller": true, "alle": true, "allant": true, "vais": true, "vas": true,
	"va": true, "allons": true, "allez": true, "vont": true, "allais": true,
	"allait": true, "allions": true, "alliez": true, "allaient": true,
	"irai": true, "iras": true, "ira": true, "irons": true, "irez": true,
	"iront": true, "irais": true, "irait": true, "irions": true, "iriez": true, "iraient": true,
	// Pouvoir
	"pouvoir": true, "pu": true, "pouvant": true, "peux": true, "peut": true,
	"pouvons": true, "pouvez": true, "peuvent": true, "pouvais": true, "pouvait": true,
	"pouvions": true, "pouviez": true, "pouvaient": true, "pourrai": true,
	"pourras": true, "pourra": true, "pourrons": true, "pourrez": true, "pourront": true,
	"pourrais": true, "pourrait": true, "pourrions": true, "pourriez": true,
	"pourraient": true, "puisse": true, "puissent": true, "puissions": true, "puissiez": true,
	// Devoir
	"devoir": true, "dois": true, "doit": true, "devons": true,
	"devez": true, "doivent": true, "devais": true, "devait": true, "devions": true,
	"deviez": true, "devaient": true, "devrai": true, "devras": true, "devra": true,
	"devrons": true, "devrez": true, "devront": true, "devrais": true, "devrait": true,
	"devrions": true, "devriez": true, "devraient": true, "doive": true,
	// Vouloir
	"vouloir": true, "voulu": true, "voulant": true, "veux": true, "veut": true,
	"voulons": true, "voulez": true, "veulent": true, "voulais": true, "voulait": true,
	"voulions": true, "vouliez": true, "voulaient": true, "voudrai": true,
	"voudras": true, "voudra": true, "voudrons": true, "voudrez": true, "voudront": true,
	"voudrais": true, "voudrait": true, "voudrions": true, "voudriez": true,
	"voudraient": true, "veuille": true, "veuillent": true,
	// Savoir
	"savoir": true, "su": true, "sachant": true, "sais": true, "sait": true,
	"savons": true, "savez": true, "savent": true, "savais": true, "savait": true,
	"savions": true, "saviez": true, "savaient": true, "saurai": true, "sauras": true,
	"saura": true, "saurons": true, "saurez": true, "sauront": true, "saurais": true,
	"saurait": true, "saurions": true, "sauriez": true, "sauraient": true,
	"sache": true, "sachent": true, "sachons": true, "sachez": true,
	// Autres verbes courants
	"dit": true, "dire": true, "disant": true, "disent": true, "dites": true,
	"disait": true, "dirait": true, "vient": true, "venir": true, "venu": true,
	"venant": true, "venez": true, "venons": true, "viennent": true,
	"prend": true, "prendre": true, "pris": true, "prenant": true, "prenez": true,
	"prenons": true, "prennent": true, "met": true, "mettre": true, "mis": true,
	"mettant": true, "mettez": true, "mettons": true, "mettent": true,
	// Locutions et mots-outils
	"voici": true, "voila": true, "sinon": true, "apres": true, "avant": true,
	"pendant": true, "n": true, "c": true, "j": true, "m": true, "t": true,
	"s": true, "etc": true,
	// Anglais fréquent (règles bilingues)
	"the": true, "and": true, "for": true, "not": true, "you": true,
	"this": true, "that": true, "with": true, "are": true, "from": true,
}

// ── Lexique du domaine jeu de société ────────────────────────────────────────
// Mots toujours inclus dans ExtractKeywords même s'ils font < 3 car.

var gameNouns = map[string]bool{
	// Composants physiques
	"plateau": true, "plateaux": true, "tuile": true, "tuiles": true,
	"carte": true, "cartes": true, "pion": true, "pions": true,
	"jeton": true, "jetons": true, "marqueur": true, "marqueurs": true,
	"cube": true, "cubes": true, "token": true, "tokens": true,
	"meeple": true, "meeples": true, "figurine": true, "figurines": true,
	"disque": true, "disques": true, "ecran": true, "ecrans": true,
	// Cartes / zones
	"deck": true, "decks": true, "pioche": true, "pioches": true,
	"defausse": true, "defausses": true, "paquet": true, "paquets": true,
	"reserve": true, "marche": true, "marches": true, "sac": true, "pile": true, "piles": true,
	// Structure de tour
	"manche": true, "manches": true, "round": true, "rounds": true,
	"phase": true, "phases": true, "etape": true, "etapes": true,
	"activation": true, "activations": true, "resolution": true,
	// Score / victoire
	"point": true, "points": true, "victoire": true, "defaite": true,
	"score": true, "scores": true, "objectif": true, "objectifs": true,
	"mission": true, "missions": true, "quete": true, "quetes": true,
	"recompense": true, "recompenses": true, "vainqueur": true, "vainqueurs": true,
	// Ressources
	"ressource": true, "ressources": true, "bois": true, "pierre": true,
	"nourriture": true, "magie": true, "energie": true, "foi": true,
	"culture": true, "influence": true, "grain": true, "ble": true,
	"fer": true, "mana": true, "monnaie": true, "monnaies": true,
	"piece": true, "pieces": true, "gemme": true, "gemmes": true,
	"cristal": true, "cristaux": true, "potion": true, "potions": true,
	// Zones / lieux
	"territoire": true, "territoires": true, "region": true, "regions": true,
	"zone": true, "zones": true, "hexagone": true, "hexagones": true,
	"case": true, "cases": true, "ile": true, "iles": true,
	"domaine": true, "domaines": true, "foret": true, "forets": true,
	"montagne": true, "montagnes": true, "desert": true, "deserts": true,
	"frontiere": true, "frontieres": true, "slot": true, "slots": true,
	// Bâtiments
	"batiment": true, "batiments": true, "ville": true, "villes": true,
	"village": true, "villages": true, "colonie": true, "colonies": true,
	"cite": true, "cites": true, "forteresse": true, "forteresses": true,
	"chateau": true, "chateaux": true, "tour": true, "tours": true,
	"temple": true, "temples": true, "mine": true, "mines": true,
	"ferme": true, "fermes": true, "port": true, "ports": true,
	"route": true, "routes": true, "chemin": true, "chemins": true,
	"caserne": true, "casernes": true, "rempart": true, "remparts": true,
	// Personnages
	"personnage": true, "personnages": true, "heros": true, "guerrier": true,
	"guerriers": true, "soldat": true, "soldats": true, "chevalier": true,
	"chevaliers": true, "archer": true, "archers": true, "explorateur": true,
	"colon": true, "colons": true, "ouvrier": true, "ouvriers": true,
	"chef": true, "leader": true, "leaders": true, "roi": true, "reine": true,
	"seigneur": true, "seigneurs": true, "mercenaire": true, "mercenaires": true,
	"garde": true, "gardes": true, "creature": true, "creatures": true,
	"monstre": true, "monstres": true, "boss": true,
	// Acteurs de jeu
	"joueur": true, "joueurs": true, "adversaire": true, "adversaires": true,
	"allie": true, "allies": true, "ennemi": true, "ennemis": true,
	"gardien": true, "gardiens": true,
	// Mécaniques
	"action": true, "actions": true, "combat": true, "attaque": true,
	"attaques": true, "defense": true, "defenses": true, "alliance": true,
	"alliances": true, "draft": true, "enchere": true, "encheres": true,
	"vote": true, "votes": true, "commerce": true, "echange": true,
	"echanges": true, "production": true, "collecte": true, "recolte": true,
	"recoltes": true, "mouvement": true, "mouvements": true,
	"deplacement": true, "deplacements": true, "placement": true,
	"recrutement": true,
	// Effets / capacités
	"effet": true, "effets": true, "capacite": true, "capacites": true,
	"pouvoir": true, "pouvoirs": true, "competence": true, "competences": true,
	"sort": true, "sorts": true, "benediction": true, "malediction": true,
	"blessure": true, "blessures": true, "buff": true, "debuff": true,
	"modificateur": true, "modificateurs": true,
	// Règles
	"regle": true, "regles": true, "exception": true, "exceptions": true,
	"contrainte": true, "contraintes": true, "restriction": true, "restrictions": true,
	"evenement": true, "evenements": true, "declencheur": true, "declencheurs": true,
	"timing": true, "priorite": true,
	// Économie
	"cout": true, "couts": true, "bonus": true, "malus": true,
	"penalite": true, "penalites": true, "limite": true, "limites": true,
	"stock": true, "stocks": true,
	// Stratégie
	"strategie": true, "strategies": true, "tactique": true, "tactiques": true,
	"combo": true, "combos": true, "synergie": true, "synergies": true,
}

func isStopword(w string) bool {
	return frenchStopwords[w]
}
