# Migration vers Clean Architecture

## 🎯 Objectif

Refactoriser le serveur Go vers une **Clean Architecture** (Uncle Bob) pour :

- ✅ Séparer la logique métier de l'infrastructure
- ✅ Faciliter les tests unitaires et d'intégration
- ✅ Améliorer la maintenabilité
- ✅ Permettre le

changement de framework sans impacter le domain

---

## 📐 Principes de la Clean Architecture

```
┌─────────────────────────────────────────────────────────┐
│                      Frameworks                          │
│    (HTTP, CLI, DB, ONNX, Redis...)                     │
│  ┌───────────────────────────────────────────────────┐  │
│  │            Interface Adapters                      │  │
│  │  (Controllers, Presenters, Repositories)          │  │
│  │  ┌─────────────────────────────────────────────┐  │  │
│  │  │          Application                         │  │  │
│  │  │      (Use Cases, DTOs)                      │  │  │
│  │  │  ┌───────────────────────────────────────┐  │  │  │
│  │  │  │         Domain                        │  │  │  │
│  │  │  │  (Entities, Interfaces, Rules)        │  │  │  │
│  │  │  └───────────────────────────────────────┘  │  │  │
│  │  └─────────────────────────────────────────────┘  │  │
│  └───────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
```

**Règle d'or** : Les dépendances pointent **vers l'intérieur**.

- Domain ne dépend de rien
- Application dépend uniquement du Domain
- Infrastructure dépend du Domain et Application
- HTTP dépend de tout

---

## 🗂️ Nouvelle structure

```
server/
├── cmd/
│   └── server/
│       └── main.go              # Point d'entrée, wire dependencies
│
├── internal/
│   ├── domain/                  # ❤️ Cœur métier (0 dépendance externe)
│   │   ├── entity/              # Entités métier
│   │   │   ├── game.go
│   │   │   ├── section.go
│   │   │   └── query.go
│   │   │
│   │   ├── repository/          # Interfaces repositories
│   │   │   ├── game_repository.go
│   │   │   ├── section_repository.go
│   │   │   └── log_repository.go
│   │   │
│   │   ├── service/             # Interfaces services métier
│   │   │   ├── embedder.go
│   │   │   ├── llm.go
│   │   │   ├── retriever.go
│   │   │   └── cache.go
│   │   │
│   │   └── errors/              # Erreurs métier
│   │       └── errors.go
│   │
│   ├── application/             # 🎯 Cas d'usage
│   │   ├── usecase/             # Use cases concrets
│   │   │   ├── ask_question.go
│   │   │   ├── import_game.go
│   │   │   ├── list_games.go
│   │   │   └── delete_game.go
│   │   │
│   │   └── dto/                 # Data Transfer Objects
│   │       ├── ask_request.go
│   │       ├── ask_response.go
│   │       └── game_dto.go
│   │
│   └── infrastructure/          # 🔧 Implémentations concrètes
│       ├── persistence/         # Accès données
│       │   ├── postgres/
│       │   │   ├── game_repository.go
│       │   │   ├── section_repository.go
│       │   │   ├── log_repository.go
│       │   │   └── migrate.go
│       │   │
│       │   └── cache/
│       │       ├── redis_cache.go
│       │       └── memory_cache.go
│       │
│       ├── ai/                  # Services IA
│       │   ├── embedder/
│       │   │   └── onnx_embedder.go
│       │   ├── llm/
│       │   │   ├── mistral_client.go
│       │   │   └── openai_client.go
│       │   └── nlp/
│       │       ├── section_detector.go
│       │       └── gameplay_extractor.go
│       │
│       ├── pipeline/            # Pipeline d'import
│       │   ├── importer.go
│       │   ├── extractor.go
│       │   └── chunker.go
│       │
│       └── retriever/           # Recherche hybride
│           └── hybrid_retriever.go
│
├── internal/interfaces/         # 🌐 Adapters
│   ├── http/                    # Interface HTTP
│   │   ├── handler/
│   │   │   ├── ask_handler.go
│   │   │   ├── game_handler.go
│   │   │   └── import_handler.go
│   │   ├── middleware/
│   │   │   ├── auth.go
│   │   │   └── ratelimit.go
│   │   └── router/
│   │       └── router.go
│   │
│   └── cli/                     # Interface CLI (future)
│       └── commands.go
│
└── pkg/                         # 📦 Utilitaires partagés
    ├── logger/
    ├── config/
    └── storage/
```
