#!/bin/sh
set -eu
umask 077

if [ "$(id -u)" = "0" ]; then
  chown -R rc:rc /data
  chmod 0700 /data
  find /data -maxdepth 1 -type f \( -name 'rc.db*' -o -name 'rc-v2.sqlite3*' \) \
    -exec chmod 0600 {} +
  if [ ! -s /data/ssh_host_ed25519_key ]; then
    ssh-keygen -q -t ed25519 -N '' -f /data/ssh_host_ed25519_key
  fi
  chown root:root /data/ssh_host_ed25519_key /data/ssh_host_ed25519_key.pub
  chmod 0600 /data/ssh_host_ed25519_key
  chmod 0644 /data/ssh_host_ed25519_key.pub
  mkdir -p /run/sshd
  /usr/sbin/sshd -f /etc/ssh/sshd_config_rc
  exec gosu rc:rc "$@"
fi

exec "$@"
