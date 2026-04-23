#!/usr/bin/env bash
set -euo pipefail

BASE_TAG="${1:-v0.13.0}"

if ! git rev-parse -q --verify "refs/tags/${BASE_TAG}" >/dev/null; then
  echo "verify-fork-surface: base tag '${BASE_TAG}' not found" >&2
  exit 1
fi

mapfile -t CHANGED_FILES < <(
  git diff "${BASE_TAG}" --name-only -- . \
    ':(exclude)*_motoko.*' ':(exclude)*_motoko_test.*' ':(exclude)FORK.md' \
    ':(exclude).agent/**' ':(exclude)make/code-health.mk' ':(exclude)scripts/verify_fork_surface.sh'
)

if [[ ${#CHANGED_FILES[@]} -eq 0 ]]; then
  echo "verify-fork-surface: no non-_motoko files changed vs ${BASE_TAG}"
  exit 0
fi

global_begin=0
global_end=0
failed=0

for file in "${CHANGED_FILES[@]}"; do
  [[ -f "$file" ]] || continue

  mapfile -t begin_lines < <(grep -n 'motoko:begin' "$file" | cut -d: -f1 || true)
  mapfile -t end_lines < <(grep -n 'motoko:end' "$file" | cut -d: -f1 || true)
  global_begin=$((global_begin + ${#begin_lines[@]}))
  global_end=$((global_end + ${#end_lines[@]}))

  if [[ ${#begin_lines[@]} -eq 0 || ${#end_lines[@]} -eq 0 ]]; then
    echo "${file}: missing motoko:begin/motoko:end fence pair" >&2
    failed=1
    continue
  fi

  # Build ordered fence ranges.
  mapfile -t fence_ranges < <(
    awk '
      /motoko:begin/ { b=NR; next }
      /motoko:end/ { if (b>0) { print b ":" NR; b=0 } }
    ' "$file"
  )

  if [[ ${#fence_ranges[@]} -eq 0 ]]; then
    echo "${file}: could not resolve balanced fence ranges" >&2
    failed=1
    continue
  fi

  # Some known migration files contain temporary larger fenced blocks.
  allow_large_and_freeform=0
  case "$file" in
    internal/pipeline/pipeline_module_compile.go|internal/testing/executor_helpers.go|internal/testing/runner.go|\
    cmd/ailang/ai_handlers.go|internal/ai/config.go|internal/ai/provider_test.go|internal/effects/io.go|\
    internal/loader/stdlib_resolver.go|internal/loader/stdlib_resolver_test.go)
      allow_large_and_freeform=1
      ;;
  esac

  # Rule 4a + 4b: line count <= 5 and allowed line shapes.
  for range in "${fence_ranges[@]}"; do
    b="${range%%:*}"
    e="${range##*:}"
    inner_count=$((e - b - 1))
    if (( allow_large_and_freeform == 0 && inner_count > 5 )); then
      echo "${file}:${b}: fenced block has ${inner_count} lines (> 5)" >&2
      failed=1
    fi

    if (( allow_large_and_freeform == 0 && inner_count > 0 )); then
      awk -v f="$file" -v b="$b" -v e="$e" '
        NR<=b || NR>=e { next }
        /^[[:space:]]*$/ { next }
        {
          ok=0
          if ($0 ~ /^[[:space:]]*RegisterEffectBuiltin\(/) ok=1
          if ($0 ~ /^[[:space:]]*RegisterOp\(/) ok=1
          if ($0 ~ /^[[:space:]]*handlers\[.*\][[:space:]]*=/) ok=1
          if ($0 ~ /^[[:space:]]*import[[:space:]]/) ok=1
          if ($0 ~ /^[[:space:]]*registerAIMotokoBuiltins\(\)/) ok=1
          if ($0 ~ /^[[:space:]]*registerIOMotokoBuiltins\(\)/) ok=1
          if ($0 ~ /^[[:space:]]*registerAIMotokoOps\(\)/) ok=1
          if ($0 ~ /^[[:space:]]*registerIOMotokoOps\(\)/) ok=1
          if ($0 ~ /^[[:space:]]*if[[:space:]]+err[[:space:]]*:=[[:space:]]*applyModelRoutingMotoko\(.*\);[[:space:]]*err[[:space:]]*!=[[:space:]]*nil[[:space:]]*\{/) ok=1
          if ($0 ~ /^[[:space:]]*return[[:space:]]+nil,[[:space:]]*err/) ok=1
          if ($0 ~ /^[[:space:]]*}[[:space:]]*$/) ok=1
          if ($0 ~ /^[[:space:]]*if[[:space:]]+motoko\.[A-Za-z0-9_]+\(.*\)[[:space:]]*\{[[:space:]]*return[[:space:]]+motoko\.[A-Za-z0-9_]+\(.*\)[[:space:]]*\}/) ok=1
          if ($0 ~ /^[[:space:]]*case[[:space:]]+".*":[[:space:]]*return[[:space:]]+motoko\.[A-Za-z0-9_]+\(.*\)/) ok=1
          if (!ok) {
            printf "%s:%d: non-whitelisted fenced line: %s\n", f, NR, $0 > "/dev/stderr"
            exit 2
          }
        }
      ' "$file" || failed=1
    fi
  done

  # Rule 2b: every changed added line in new file must be marker-or-fenced.
  while IFS= read -r line_no; do
    [[ -n "$line_no" ]] || continue
    line_text="$(sed -n "${line_no}p" "$file")"
    if [[ "$line_text" =~ ^[[:space:]]*$ ]]; then
      continue
    fi
    if [[ "$line_text" =~ ^[[:space:]]*// ]] || [[ "$line_text" =~ ^[[:space:]]*# ]]; then
      continue
    fi
    covered=0
    for range in "${fence_ranges[@]}"; do
      b="${range%%:*}"
      e="${range##*:}"
      inner_start=$((b + 1))
      inner_end=$((e - 1))
      if (( line_no == b || line_no == e || (line_no >= inner_start && line_no <= inner_end) )); then
        covered=1
        break
      fi
    done
    if (( covered == 0 )); then
      echo "${file}:${line_no}: changed line outside motoko fences" >&2
      failed=1
    fi
  done < <(
    git diff -U0 "${BASE_TAG}" -- "$file" | awk '
      /^@@/ {
        line = $0
        sub(/^.*\+/, "", line)
        sub(/ .*/, "", line)
        split(line, a, ",")
        next_new = a[1] + 0
        next
      }
      /^\+\+\+/ { next }
      /^\+/ { print next_new; next_new++; next }
      /^-/ { next }
      /^ / { next_new++; next }
    '
  )
done

if [[ "$global_begin" -ne "$global_end" ]]; then
  echo "global fence imbalance: begin=${global_begin} end=${global_end}" >&2
  failed=1
fi

if [[ "$failed" -ne 0 ]]; then
  echo "verify-fork-surface: FAILED" >&2
  exit 1
fi

echo "verify-fork-surface: PASS"
