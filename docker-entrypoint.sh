#!/bin/sh
set -eu
umask 077

if [ "$(id -u)" = "0" ]; then
  chown -R rc:rc /data
  chmod 0700 /data
  find /data -maxdepth 1 -type f \( -name 'rc.db*' -o -name 'rc-v2.sqlite3*' \) \
    -exec chmod 0600 {} +
  exec gosu rc:rc "$@"
fi

exec "$@"
