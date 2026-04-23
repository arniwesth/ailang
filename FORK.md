# FORK

## Base
- Upstream base tag: `v0.13.0`
- Upstream base commit: `99f76ec7ac724ef4cf57a2f29a663d592775cecf`
- Motoko branch: `motoko`
- Rebase baseline target: `origin/dev` @ `789192f95cfc20f873e635b721b2875d494b3b9c` (measured 2026-04-21)

## _motoko Files Inventory
- `internal/ai/openai/endpoint_motoko.go`
  - Purpose: `openai://host:port/model` URI parsing + local endpoint routing + typed provider error classification.
  - Key exports: `parseEndpointModelMotoko`, `routeEndpointModelMotoko`, `classifyOpenAIErrorMotoko`.
  - Upstream interfaces touched: `internal/ai.Request`, `openai.Client`, `ai.ProviderError`.
- `internal/ai/openai/openrouter_motoko.go`
  - Purpose: model-prefix routing for `openrouter/` and `openai/`, optional `OPENAI_BASE_URL` override.
  - Key exports: `routeOpenRouterMotoko`, `routeOpenAIPrefixMotoko`, `applyModelRoutingMotoko`.
  - Upstream interfaces touched: OpenAI client generation path in `internal/ai/openai/client.go`.
- `internal/ai/openai/stream_motoko.go`
  - Purpose: OpenAI chat-completions SSE streaming path and stream chunk emission.
  - Key exports: `(*Client).GenerateStream`, `readSSEDataMotoko`.
  - Upstream interfaces touched: `ai.StreamingProvider`, `ai.StreamHandler`, OpenAI client request/response layer.
- `internal/ai/openai/endpoint_motoko_test.go`
  - Purpose: local endpoint URI parsing and error classification coverage.
- `internal/ai/openai/openrouter_motoko_test.go`
  - Purpose: OpenRouter/OpenAI prefix routing and base-url normalization coverage.
- `internal/ai/openai/stream_motoko_test.go`
  - Purpose: SSE parsing and stream lifecycle behavior coverage.
- `internal/ai/stream_motoko.go`
  - Purpose: Motoko-side streaming abstraction (`StreamEvent`, `StreamingProvider`, `CallStream` fallback semantics).
  - Key exports: `StreamEvent`, `StreamingProvider`, `(*Handler).CallStream`.
  - Upstream interfaces touched: `internal/ai/handler.go` runtime handler object.
- `internal/builtins/ai_motoko.go`
  - Purpose: registers Result/stream AI builtins in `std/ai_motoko`.
  - Key exports: `registerAIMotokoBuiltins`, `_ai_call_result`, `_ai_call_json_result`, `_ai_call_stream_result` impls.
  - Upstream interfaces touched: builtin registry (`RegisterEffectBuiltin`), AI effect dispatcher.
- `internal/builtins/ai_motoko_test.go`
  - Purpose: Result/stream builtin type and behavior checks.
- `internal/builtins/io_motoko.go`
  - Purpose: registers `_io_poll_stdin` builtin in `std/io_motoko`.
  - Key exports: `registerIOMotokoBuiltins`.
  - Upstream interfaces touched: builtin registry + IO effect dispatch.
- `internal/builtins/io_motoko_test.go`
  - Purpose: non-blocking stdin poll behavior tests.
- `internal/effects/ai_motoko.go`
  - Purpose: `AI.callResult`, `AI.callJsonResult`, `AI.callStreamResult` effect ops, stream event emission, abort handling, typed error mapping.
  - Key exports: `registerAIMotokoOps`, `aiCallStreamResultMotoko`.
  - Upstream interfaces touched: effect registry, `EffContext`, AI handler, trace/event emission.
- `internal/effects/ai_motoko_test.go`
  - Purpose: streaming ordering, abort, and Result error-path tests.
- `internal/effects/io_motoko.go`
  - Purpose: `IO.pollStdin` effect op implementation over buffered stdin.
  - Key exports: `registerIOMotokoOps`, `ioPollStdinMotoko`.
  - Upstream interfaces touched: IO effect registry and `EffContext` IO reader.
- `std/ai_motoko.ail`
  - Purpose: Motoko-facing AI Result/stream API surface.
  - Key exports: `AIError`, `AIStreamChunk`, `AIStreamResult`, `callResult`, `callJsonResult`, `callStreamResult`.
  - Upstream interfaces touched: builtins `_ai_call_result`, `_ai_call_json_result`, `_ai_call_stream_result`.
- `std/io_motoko.ail`
  - Purpose: Motoko-facing non-blocking stdin polling API.
  - Key exports: `pollStdin`.
  - Upstream interfaces touched: builtin `_io_poll_stdin`.

## Fenced Edits Inventory
- `internal/builtins/ai.go:15-17`
  - Purpose: register Motoko AI builtins.
- `internal/builtins/io.go:23-25`
  - Purpose: register Motoko IO builtins.
- `internal/effects/ai.go:172-174`
  - Purpose: register Motoko AI effect ops.
- `internal/effects/io.go:11-18`
  - Purpose: import Motoko stdlib IO module map entry.
- `internal/effects/io.go:27-29`
  - Purpose: register Motoko IO effect ops.
- `internal/effects/io.go:57-61`
  - Purpose: IO handler dispatch hook to Motoko stdlib module.
- `internal/effects/io.go:90-94`
  - Purpose: IO operation exposure includes Motoko poll op.
- `internal/effects/io.go:194-196`
  - Purpose: IO operation map registration extension.
- `internal/ai/openai/client.go:73-77`
  - Purpose: apply Motoko model/endpoint routing before provider call.
- `cmd/ailang/ai_handlers.go:24-37`
  - Purpose: Motoko runtime flag/env wiring for handler behavior.
- `cmd/ailang/ai_handlers.go:91-99`
  - Purpose: handler selection compatibility branch.
- `cmd/ailang/ai_handlers.go:159-167`
  - Purpose: provider setup compatibility branch.
- `cmd/ailang/ai_handlers.go:203-205`
  - Purpose: Motoko fallback handling.
- `internal/ai/config.go:36-40`
  - Purpose: compatibility env mapping fence.
- `internal/ai/provider_test.go:119-123`
  - Purpose: provider test compatibility hook.
- `internal/loader/stdlib_resolver.go:21-23`
  - Purpose: import alias compatibility fence.
- `internal/loader/stdlib_resolver.go:306-308`
  - Purpose: stdlib module map extension for Motoko modules.
- `internal/loader/stdlib_resolver.go:316-336`
  - Purpose: resolver fallback compatibility path.
- `internal/loader/stdlib_resolver_test.go:406-432`
  - Purpose: resolver test compatibility assertions.
- `internal/testing/executor_helpers.go:28-82`
  - Purpose: execution harness compatibility shim for Motoko module naming/constructors.
- `internal/testing/executor_helpers.go:98-105`
  - Purpose: module-qualified lookup preference.
- `internal/testing/executor_helpers.go:334-367`
  - Purpose: imported-module constructor/environment compatibility injection.
- `internal/testing/executor_helpers.go:390-396`
  - Purpose: pending lambda binding compatibility struct.
- `internal/testing/executor_helpers.go:409-417`
  - Purpose: module path env binding compatibility branch.
- `internal/testing/executor_helpers.go:423-425`
  - Purpose: additional module-qualified env injection.
- `internal/testing/executor_helpers.go:432-440`
  - Purpose: binding writeback compatibility branch.
- `internal/testing/executor_helpers.go:448-452`
  - Purpose: qualified/unqualified symbol storage compatibility path.
- `internal/testing/executor_helpers.go:461-465`
  - Purpose: pending-binding module path handling.
- `internal/testing/runner.go:73-84`
  - Purpose: test tuple actuals compatibility handling.
- `internal/pipeline/pipeline_module_compile.go:639-663`
  - Purpose: module compile/export compatibility fence.

## Effect-Set Changes
- Phase 5 audit command: `ailang check src/core/*.ail` (see `.agent/reports/phase5_effect_audit_core_check.log`) passed.
- Before migration (legacy path): streaming-specific flow did not require explicit `Stream` on Motoko runtime entry points.
- After migration:
  - `src/core/rpc.ail` streaming paths now type-check with `Stream` included where stream primitives are used.
  - Top-level `main` effect set is `! {Net, AI, SharedMem, IO, Env, FS, Clock, Process, Stream}`.
  - Parent `Makefile` run targets were updated to include `Stream` in caps for parity.

## Rebase Playbook
1. `cd ailang && git fetch --tags upstream origin`
2. `git checkout -b motoko-vX.Y.Z <new-upstream-tag>`
3. Cherry-pick Motoko commits from `motoko` (prefer smallest logical commits first).
4. Resolve conflicts with this order:
   - shared files: keep upstream, then re-apply only fenced Motoko edits
   - `_motoko` files: keep Motoko implementation
5. Run fork-surface verification:
   - `make verify-fork-surface FORK_BASE_TAG=<new-upstream-tag>`
6. Run runtime and integration validation:
   - `go test ./...`
   - parent repo Phase 5 checks (`make test_core`, core effect audit, baseline trace diff)
7. Update this document:
   - Base tag/SHA
   - `_motoko` inventory
   - fenced line ranges
   - change log row with conflict count and issues found

## Known Risk Areas
- `internal/ai/openai/client.go`: provider dispatch hook churn can break routing fences.
- `internal/ai/openai/*`: upstream API-type routing changes can invalidate stream/endpoint wrappers.
- `internal/builtins/{ai,io}.go`: builtin registration entrypoints are sensitive to upstream refactors.
- `internal/effects/{ai,io}.go`: effect op registration layout changes can move fence anchors.
- `internal/loader/stdlib_resolver.go`: stdlib resolver internals are a likely churn point for custom modules.
- `internal/testing/*` and `internal/pipeline/pipeline_module_compile.go`: compatibility fences are broad and should be reduced over time.

## Change Log
- `2026-04-21`: Rebase-forward migration landed on base `v0.13.0` (`99f76ec7ac724ef4cf57a2f29a663d592775cecf`).
- `2026-04-21`: Legacy branch tagged `pre-rebase-forward` at `dev_agent` head (`fe241099`).
- `2026-04-21`: Added `make verify-fork-surface` + `scripts/verify_fork_surface.sh`.
- `2026-04-21`: Dry-run rebase baseline onto `origin/dev` (`789192f95cfc20f873e635b721b2875d494b3b9c`) completed with `0` conflicts (`(none)`).
- `2026-04-21`: Normalized all fence terminators to `motoko:end` (removed legacy `motoko:endtoto` markers).
