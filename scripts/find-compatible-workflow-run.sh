#!/bin/sh
set -eu

if [ "$#" -lt 3 ]; then
  echo "usage: $0 WORKFLOW TARGET_SHA PATH..." >&2
  exit 2
fi
: "${GH_TOKEN:?GH_TOKEN is required}"
: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"

workflow=$1
target=$2
shift 2

temporary=$(mktemp)
trap 'rm -f "$temporary"' EXIT HUP INT TERM
wait_seconds=${RC_WORKFLOW_WAIT_SECONDS:-2400}
poll_seconds=${RC_WORKFLOW_POLL_SECONDS:-15}
deadline=$(( $(date +%s) + wait_seconds ))
tab=$(printf '\t')

while [ "$(date +%s)" -lt "$deadline" ]; do
  gh api --paginate --method GET \
    "repos/$GITHUB_REPOSITORY/actions/workflows/$workflow/runs" \
    -f branch=main \
    -f event=push \
    -f status=completed \
    -f per_page=100 \
    --jq '.workflow_runs[] | select(.conclusion == "success") | [.id, .head_sha] | @tsv' \
    > "$temporary"

  while IFS="$tab" read -r run_id head_sha; do
    [ -n "$run_id" ] || continue
    if ! git cat-file -e "$head_sha^{commit}" 2>/dev/null; then
      git fetch --quiet origin "$head_sha" || continue
    fi
    git merge-base --is-ancestor "$head_sha" "$target" || continue
    if git diff --quiet "$head_sha" "$target" -- "$@"; then
      printf '%s\n' "$run_id"
      exit 0
    fi
  done < "$temporary"

  echo "waiting for a compatible successful $workflow run for $target" >&2
  sleep "$poll_seconds"
done

echo "timed out waiting for a compatible successful $workflow run for $target" >&2
exit 1
