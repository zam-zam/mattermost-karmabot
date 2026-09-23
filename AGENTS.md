# AI Agent Instructions (AGENTS.md)

This document provides context, style guidelines, and execution guardrails for AI coding agents working on this repository. Follow these instructions strictly.

## Project Overview
* **Language:** Go (Golang)
* **Minimum Go Version:** 1.26+
* **Project Type:** Messenger bot
* **Project Description:** Bot for giving other users karma points
* **Key Dependencies:**
  * github.com/joho/godotenv and github.com/kelseyhightower/envconfig for config
  * official mattermost libs github.com/mattermost/*

## Expected Project Structure
Maintain idiomatic Go layout structures:
```text
├── cmd/
│   └── <appname>/
│       └── main.go       # Application entry point
├── internal/             # Private application and business logic
├── pkg/                  # Public library code (safe for other projects to import)
├── go.mod
├── go.sum
├── env.example           # Example env file
└── AGENTS.md             # This file
```

## Go Style Guidelines
* **Error Handling:** Always handle errors explicitly. Do not drop errors using `_`. Return errors wrapped with meaningful context using `fmt.Errorf("context: %w", err)`.
* **Formatting:** Always run `go fmt ./...` and `go vet ./...` before considering a task complete.
* **Concurrency:** Use channels and `sync` primitives only when necessary. Ensure goroutines have an explicit lifecycle and are context-aware (`context.Context`).
* **Panics:** Do not use `panic()` for control flow or recoverable errors. Use `panic` only for unrecoverable initialization failures.

## Testing & Verification
* **Test Location:** Keep tests in `*_test.go` files in the same directory as the source code being tested.
* **Execution:** Before declaring success on any modification, you must execute:
  ```bash
  go test -v ./...
  ```
* **Coverage:** New features must include basic unit tests covering happy paths and primary failure modes.

## Guardrails & Critical Constraints
* **Destructive Actions:** **NEVER** delete local database files (`.db`, `.sqlite`), migrations, or configuration files without explicit human confirmation.
* **Dependencies:** Do not introduce new external dependencies via `go get` unless explicitly requested by the user or required to satisfy a core functionality requirement.
* **Code Modification:** Do not rewrite whole files if a minor localized change is sufficient. Keep git diffs minimal and highly readable.

## Documentation Guidilines
* **Docs Location:** Keep docs in README.md file
* **Configuration docs:** Document all configurable params with their purpose and default values in the follwing table format `Param | default value | description`  
* **Docs Freshness:**  If a new feature or command-line flag is introduced, update the root `README.md`
* **Docs Language** Write docs in english and russian languages. Files names: `README.md` for english, `README-RU.md` for russian

## Commit Guidelines
You are allowed to make atomic commits only when a complete logical unit of work is done and verified.
* **Commit Message Format:** Follow Conventional Commits (e.g., `feat: add user authentication endpoint` or `fix: resolve pointer panic in parsing`).
* **Condition:** Do not commit code that breaks `go test ./...` or fails to compile.
