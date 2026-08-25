#!/bin/sh
set -eu

if [ "$(id -u)" = "0" ]; then
  chown -R rc:rc /data
  chmod 0700 /data
  find /data -maxdepth 1 -type f -name 'rc.db*' -exec chmod 0600 {} +
  exec su-exec rc:rc "$@"
fi

exec "$@"
