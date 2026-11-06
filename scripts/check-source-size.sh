#!/bin/sh
set -eu

limit="${RC_SOURCE_LINE_LIMIT:-300}"

violations="$(
  for directory in web scripts components wit pkg cmd; do
    [ -d "$directory" ] || continue
    find "$directory" \
      -type f \
      \( -name '*.ts' -o -name '*.tsx' -o -name '*.wit' -o -name '*.go' \) \
      ! -name 'wit_exports.go' \
      ! -name '*wit_bindings.go' \
      -exec wc -l {} +
  done | awk -v limit="$limit" '$2 != "total" && $1 > limit { print $1 " " $2 }'
)"

if [ -n "$violations" ]; then
  echo "maintained source files must not exceed $limit lines:" >&2
  printf '%s\n' "$violations" >&2
  exit 1
fi

printf 'source-size limit: %s lines\n' "$limit"
