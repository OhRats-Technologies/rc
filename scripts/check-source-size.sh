#!/bin/sh
set -eu

limit="${RC_SOURCE_LINE_LIMIT:-300}"
violations="$({
  find crates web scripts -type f \
    \( -name '*.rs' -o -name '*.ts' -o -name '*.tsx' \) \
    -exec wc -l {} +
} | awk -v limit="$limit" '$2 != "total" && $1 > limit { print $1 " " $2 }')"

if [ -n "$violations" ]; then
  echo "maintained source files must not exceed $limit lines:" >&2
  printf '%s\n' "$violations" >&2
  exit 1
fi

printf 'source-size limit: %s lines\n' "$limit"
