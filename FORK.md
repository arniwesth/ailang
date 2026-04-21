# FORK

## Base
- Upstream base tag: `v0.13.0`
- Upstream base commit: `99f76ec7ac724ef4cf57a2f29a663d592775cecf`
- Motoko branch: `motoko`

## _motoko Files Inventory
- `internal/ai/openai/endpoint_motoko.go`
- `internal/ai/openai/endpoint_motoko_test.go`
- `internal/ai/openai/openrouter_motoko.go`
- `internal/ai/openai/openrouter_motoko_test.go`
- `internal/ai/openai/stream_motoko.go`
- `internal/ai/openai/stream_motoko_test.go`
- `internal/ai/stream_motoko.go`
- `internal/builtins/ai_motoko.go`
- `internal/builtins/ai_motoko_test.go`
- `internal/builtins/io_motoko.go`
- `internal/builtins/io_motoko_test.go`
- `internal/effects/ai_motoko.go`
- `internal/effects/ai_motoko_test.go`
- `internal/effects/io_motoko.go`
- `std/ai_motoko.ail`
- `std/io_motoko.ail`

## Fenced Edits Inventory
- `internal/builtins/ai.go` (Motoko AI builtin registration)
- `internal/builtins/io.go` (stdin polling builtin registration)
- `internal/effects/ai.go` (streaming AI op registration)
- `internal/effects/io.go` (stdin polling effect op registration)
- `internal/ai/openai/client.go` (OpenRouter/local endpoint dispatch)
- `internal/testing/executor_helpers.go` (Motoko test harness/runtime resolver compatibility fences)
- `internal/testing/runner.go` (Motoko inline harness selection fence)
- `internal/pipeline/pipeline_module_compile.go` (Motoko loader map compatibility fence)

## Effect-Set Changes
- Parent runtime effect audit (Phase 5): `src/core/*.ail` type-checks cleanly.
- `src/core/rpc.ail` now explicitly includes `Stream` in streaming paths and top-level `main`.
- Capability flags updated where needed in parent repo (`Makefile` dummy-extension targets) to include `Stream`.

## Rebase Playbook
1. Create a new branch from next upstream release tag.
2. Cherry-pick Motoko commits.
3. Resolve conflicts by preferring upstream in shared files and re-applying only fenced Motoko edits.
4. Run:
   - `go test ./...`
   - parent `make test_core`
   - fork surface verification (`make verify-fork-surface`, Phase 6 target)
5. Update this file inventories and change log.

## Change Log
- `2026-04-21`: Rebase-forward migration to `v0.13.0` completed through Phase 5 validation.
