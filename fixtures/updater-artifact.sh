#!/bin/sh
if [ "${1:-}" = "--version" ]; then
  printf '%s\n' 'RC kernel 0.1.1'
  exit 0
fi
exit 1
