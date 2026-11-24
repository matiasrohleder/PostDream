#!/bin/sh
# PostDream standalone entrypoint: runs a private PostgreSQL inside the container, then PostDream.
#
# - Data lives in /data (mount a volume there to keep it; without one, everything goes away with the container).
# - PostgreSQL only listens on a Unix socket: it is never reachable from outside the container.
# - Setting DATABASE_URL skips the embedded database and uses yours instead.
set -eu

PGDATA="${PGDATA:-/data/postgres}"
PGSOCKET="${PGSOCKET:-/run/postgresql}"
export PGDATA

log() { echo "standalone: $*" >&2; }

pg_started=""
if [ -z "${DATABASE_URL:-}" ]; then
	mkdir -p "$PGDATA" "$PGSOCKET"
	chmod 700 "$PGDATA"
	if [ ! -s "$PGDATA/PG_VERSION" ]; then
		log "initializing the embedded database in $PGDATA"
		initdb --auth=trust --username=postdream --encoding=UTF8 --no-instructions -D "$PGDATA" >/dev/null
	fi
	# synchronous_commit=off: a crash may lose the last few emails, never corrupts the database. Good trade for a sandbox.
	pg_ctl -D "$PGDATA" -w -t 60 -l "$PGDATA/../postgres.log" \
		-o "-c listen_addresses='' -c unix_socket_directories='$PGSOCKET' -c synchronous_commit=off" start >/dev/null
	pg_started=1
	if ! psql -h "$PGSOCKET" -U postdream -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname = 'postdream'" | grep -q 1; then
		createdb -h "$PGSOCKET" -U postdream postdream
	fi
	export DATABASE_URL="postgres:///postdream?host=$PGSOCKET&user=postdream&sslmode=disable"
	log "embedded database ready"
fi

stop_db() {
	if [ -n "$pg_started" ]; then
		pg_ctl -D "$PGDATA" -m fast -w stop >/dev/null 2>&1 || true
	fi
}

postdream "$@" &
pid=$!
trap 'kill -TERM "$pid" 2>/dev/null || true' TERM INT

set +e
wait "$pid"
status=$?
# A trapped signal interrupts wait before PostDream has finished shutting down: wait for it for real.
if kill -0 "$pid" 2>/dev/null; then
	wait "$pid"
	status=$?
fi
set -e

stop_db
exit "$status"
