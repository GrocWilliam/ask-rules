/**
 * types.ts — Interfaces TypeScript pour l'analyse de règles de jeu de société
 */

/** Catégorie sémantique d'une section de règles */
export type GameSectionType =
  | 'presentation'
  | 'but_du_jeu'
  | 'materiel'
  | 'preparation'
  | 'tour_de_jeu'
  | 'cartes_evenement'
  | 'regles_speciales'
  | 'victoire'
  | 'variante'
  | 'conseils'
  | 'autre';

/** Résultat brut du découpage en sections (avant enrichissement) */
export interface RawSection {
  titre: string;
  contenu: string;
  /** Niveau hiérarchique : 1 = chapitre principal, 2 = sous-section, 3 = sous-sous-section */
  niveau: 1 | 2 | 3;
  /** Numéro de la première page du contenu (PDF uniquement, undefined pour TXT) */
  page_debut?: number;
  /** Numéro de la dernière page du contenu (PDF uniquement, undefined pour TXT) */
  page_fin?: number;
}

/** Résultat de l'analyse NLP française d'une section */
export interface NlpResult {
  /** Noms clés du jeu : pions, cartes, ressources, lieux... */
  entites: string[];
  /** Verbes d'action à l'infinitif : placer, piocher, attaquer... */
  actions: string[];
  /** Résumé extractif (2 premières phrases significatives) */
  resume: string;
}

/** Mécaniques de jeu détectées dans une section */
export type GameMechanic =
  // ── Mécaniques existantes ──────────────────────────────────────────────────
  | 'placement'
  | 'pioche'
  | 'gestion_ressources'
  | 'combat_des'
  | 'controle_territoire'
  | 'commerce'
  | 'draft_cartes'
  | 'points_victoire'
  | 'cooperation'
  | 'events'
  | 'encheres'
  | 'construction'
  | 'deduction'
  | 'mouvement'
  | 'gestion_main'
  | 'programmation'
  | 'role_secret'
  // ── Nouvelles mécaniques ───────────────────────────────────────────────────
  /** Lancer des dés (résolution générique, hors combat) */
  | 'lancer_des'
  /** Pose d'ouvrier sur une action limitée (worker placement) */
  | 'pose_ouvrier'
  /** Passer son tour ou une action */
  | 'passer_tour'
  /** Récolter / collecter le fruit d'une action préalable */
  | 'recolte'
  /** Exploration de tuiles, révélation progressive du plateau */
  | 'exploration'
  /** Combos / enchaînement d'actions (engine building léger) */
  | 'combo_actions'
  /** Rotation / sélection d'actions par roue ou cadran */
  | 'roue_actions'
  /** Enchères en temps réel ou par placement de jetons */
  | 'encheres_placement'
  /** Collecte et scoring de majorités ou de sets */
  | 'majorite_sets'
  /** Gestion de la durée (sablier, round limité, compte à rebours) */
  | 'course_temps';

/** Profil de gameplay complet d'un jeu, extrait depuis ses règles */
export interface MechanicEvidence {
  /** Valeur de la mécanique détectée */
  mechanic: GameMechanic;
  /**
   * Libellé lisible en français
   * Ex: "Pose d'ouvrier", "Lancer les dés"
   */
  label: string;
  /** Nombre de sections où cette mécanique apparaît */
  occurrences: number;
  /** Titres des sections les plus représentatives */
  sections_sources: string[];
  /**
   * Score de confiance entre 0 et 1 :
   *   occurrences / nombre_total_sections (plafonné à 1)
   */
  confidence: number;
}

export interface GameplayProfile {
  /** Mécaniques primaires : confidence ≥ 0.15 ou occurrences ≥ 3 */
  primaires: MechanicEvidence[];
  /** Mécaniques secondaires : au moins 1 occurrence mais sous le seuil primaire */
  secondaires: MechanicEvidence[];
  /** Toutes les mécaniques triées par nombre d'occurrences décroissant */
  toutes: MechanicEvidence[];
}

// ── Actions joueur ────────────────────────────────────────────────────────────

/**
 * Une action concrète qu'un joueur peut ou doit effectuer.
 * Extraite par analyse des phrases des règles (patterns verbaux français).
 */
export interface PlayerAction {
  /** Identifiant stable : slug de "verbe_objet" */
  id: string;
  /** Libellé court lisible (ex: "Poser un ouvrier") */
  label: string;
  /** Verbe d'action normalisé à l'infinitif (ex: "placer") */
  verbe: string;
  /** Complément principal de l'action (ex: "un ouvrier sur une case") */
  objet: string;
  /** Phrase(s) source extraite(s) des règles décrivant l'action */
  description: string;
  /** Phase(s) du jeu où cette action est disponible */
  phases: string[];
  /** Conditions/coûts textuels pour réaliser l'action */
  conditions: string[];
  /** Effets textuels produits par l'action */
  effets: string[];
  /** Mécanique de jeu associée (si détectable) */
  mecanique?: GameMechanic;
  /** Nombre d'occurrences dans le rulebook */
  occurrences: number;
  /** Titre de la section source principale */
  section_source: string;
  /** Page source (PDF) */
  page_source?: number;
}

// ── Graphe de dépendances ─────────────────────────────────────────────────────

/**
 * Nature de la relation entre deux actions.
 *
 * - `requiert`    : A ne peut pas être réalisée sans que B ait eu lieu
 * - `active`      : A débloque ou permet B
 * - `precede`     : A doit se faire avant B (séquence explicite)
 * - `exclut`      : A et B ne peuvent pas coexister dans le même tour
 * - `produit_pour`: A produit une ressource/état nécessaire à B
 */
export type DependencyType =
  | 'requiert'
  | 'active'
  | 'precede'
  | 'exclut'
  | 'produit_pour';

/** Arc orienté dans le graphe de dépendances entre actions */
export interface ActionEdge {
  /** id de l'action source */
  source: string;
  /** id de l'action cible */
  target: string;
  /** Nature de la relation */
  type: DependencyType;
  /** Libellé court de la relation (ex: "après avoir placé") */
  label?: string;
  /** Extrait textuel de la règle justifiant cette relation */
  evidence?: string;
}

/** Graphe complet des dépendances entre actions joueur */
export interface ActionGraph {
  /** IDs des actions du graphe (référence vers FullGameplayAnalysis.actions) */
  noeuds: string[];
  /** Tous les arcs (dépendances) du graphe */
  aretes: ActionEdge[];
}

// ── Flux de jeu ───────────────────────────────────────────────────────────────

/** Catégorie d'une phase dans le déroulement de la partie */
export type GamePhaseType =
  | 'mise_en_place'
  | 'debut_partie'
  | 'debut_manche'
  | 'tour_joueur'
  | 'fin_manche'
  | 'fin_partie'
  | 'autre';

/** Une phase du déroulement de la partie */
export interface GamePhase {
  /** Identifiant slug */
  id: string;
  /** Libellé lisible (ex: "Phase 1 — Placement des ouvriers") */
  label: string;
  /** Position dans la séquence (0 = premier) */
  ordre: number;
  /** Catégorie sémantique */
  type: GamePhaseType;
  /** Description extraite des règles */
  description: string;
  /** Ids des actions disponibles pendant cette phase */
  actions_disponibles: string[];
  /** Vrai si la fin de cette phase peut déclencher la fin de partie */
  est_declencheur_fin: boolean;
  /** Titre de la section source */
  section_source: string;
  /** Page source */
  page_source?: number;
}

/** Structure complète du déroulement d'une partie */
export interface GameFlow {
  /** Phases dans l'ordre chronologique */
  phases: GamePhase[];
  /** Description textuelle du tour type (résumé extrait des règles) */
  structure_tour: string;
  /** Conditions textuelles déclenchant la fin de partie */
  declencheurs_fin: string[];
  /** Nombre de manches fixe (null si variable ou inconnu) */
  nombre_manches: number | null;
  /** La partie se joue-t-elle joueur par joueur (true) ou simultanément (false) */
  est_tour_par_tour: boolean;
}

// ── Conditions de victoire ────────────────────────────────────────────────────

/**
 * Type de condition de victoire.
 *
 * - `points`      : décompte de points à la fin
 * - `objectif`    : atteindre un objectif spécifique
 * - `elimination` : éliminer les adversaires
 * - `course`      : premier à atteindre un seuil
 * - `cooperation` : victoire/défaite collective
 * - `survie`      : rester le dernier en jeu
 */
export type VictoryType =
  | 'points'
  | 'objectif'
  | 'elimination'
  | 'course'
  | 'cooperation'
  | 'survie';

/** Une condition permettant de gagner ou de perdre la partie */
export interface VictoryCondition {
  /** Nature de la victoire */
  type: VictoryType;
  /** Description textuelle extraite des règles */
  description: string;
  /** Seuil quantitatif si applicable (ex: "10 points", "3 villes") */
  seuil?: string;
  /** Vrai si c'est une condition de défaite plutôt que de victoire */
  est_defaite: boolean;
  /** Titre de la section source */
  section_source: string;
  /** Page source */
  page_source?: number;
}

// ── Analyse gameplay complète ─────────────────────────────────────────────────

/**
 * Analyse structurelle complète du gameplay d'un jeu.
 * Étend le profil de mécaniques avec les actions, le graphe, le flux et la victoire.
 */
export interface FullGameplayAnalysis extends GameplayProfile {
  /** Actions concrètes disponibles pour le joueur */
  actions: PlayerAction[];
  /** Graphe de dépendances entre les actions */
  graph: ActionGraph;
  /** Flux de jeu du début à la fin de partie */
  flux: GameFlow;
  /** Conditions de victoire (et de défaite) */
  victoire: VictoryCondition[];
}

/** Section enrichie : contenu + NLP + type + mécaniques */
export interface GameSection extends RawSection, NlpResult {
  type_section: GameSectionType;
  mecaniques: GameMechanic[];
  embedding?: number[] | null;
}

/** Métadonnées extraites de l'en-tête du document */
export interface GameMetadata {
  joueurs_min: number | null;
  joueurs_max: number | null;
  age_minimum: number | null;
  duree_minutes_min: number | null;
  duree_minutes_max: number | null;
}

/** Statistiques globales de l'analyse */
export interface Statistics {
  caracteres: number;
  mots: number;
  sections: number;
  entites_total: number;
  actions_total: number;
  mecaniques_detectees: GameMechanic[];
}

/** Structure JSON de sortie complète */
export interface GameAnalysisResult {
  jeu: string;
  fichier: string;
  date_analyse: string;
  metadata: GameMetadata;
  statistiques: Statistics;
  gameplay: FullGameplayAnalysis;
  sections: GameSection[];
}

// ── Base de connaissance (PostgreSQL) ─────────────────────────────────────────

/** Section enrichie d'un identifiant unique, stockée en base */
export interface StoredSection extends GameSection {
  /** Identifiant unique : "{jeu_slug}_{index}" */
  section_id: string;
  /** Chemin hiérarchique complet (ex: "MATÉRIEL > Cartes") */
  hierarchy_path?: string;
  /** Index du chunk pour cette section (0, 1, 2...) */
  chunk_index?: number;
  /** Nombre total de chunks pour cette section */
  total_chunks?: number;
}

/** Entrée dans la base de connaissance pour un jeu */
export interface KnowledgeBaseEntry {
  /** Slug du nom du jeu (ex: "les-gardiens-du-royaume") */
  id: string;
  jeu: string;
  fichier: string;
  date_ajout: string;
  metadata: Partial<GameMetadata>;
  statistiques: Partial<Statistics>;
  gameplay?: FullGameplayAnalysis;
  sections: StoredSection[];
}

/** Résultat d'une récupération sémantique */
export interface ScoredSection {
  score: number;
  section: StoredSection;
  jeu: string;
  jeu_id: string;
}

/** Coût en tokens d'un appel LLM */
export interface LLMTokenUsage {
  /** Tokens du prompt (contexte + question) */
  prompt: number;
  /** Tokens générés dans la réponse */
  completion: number;
  /** Total (prompt + completion) */
  total: number;
}

/** Résultat d'une requête LLM */
export interface LLMResponse {
  answer: string;
  model: string;
  used_llm: boolean;
  /** Usage en tokens (absent si le fournisseur ne le renvoie pas ou sans LLM) */
  tokens?: LLMTokenUsage;
}
