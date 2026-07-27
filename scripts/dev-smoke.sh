#!/usr/bin/env bash
# SD smoke check — this script IS the spec for `make dev` (PLAN §SD):
#   1. `make dev` exits 0: Postgres up, migrations applied, bin/vantage built.
#   2. `vantage target list` runs against the dev DB with no manual env setup
#      (the .env `make dev` created is the only config source).
#   3. A second `make dev` on the already-up env is idempotent: exit 0, fast,
#      no re-create, no duplicate migrations.
# Run from anywhere: scripts/dev-smoke.sh. Needs Go + Docker, nothing else.
set -euo pipefail
cd "$(dirname "$0")/.."

echo "== dev-smoke 1/3: make dev from current state =="
make dev

echo "== dev-smoke 2/3: CLI reaches the dev DB (no manual env) =="
set -a
. ./.env
set +a
./bin/vantage target list

echo "== dev-smoke 3/3: second make dev is idempotent =="
start=$(date +%s)
make dev
elapsed=$(( $(date +%s) - start ))
# "Quickly" = no image pull, no container re-create, no re-migration; the
# threshold is generous so CI jitter doesn't flake it.
if [ "$elapsed" -gt 60 ]; then
	echo "dev-smoke: FAIL — idempotent re-run took ${elapsed}s (>60s), not a no-op" >&2
	exit 1
fi

echo "dev-smoke: OK (re-run ${elapsed}s)"
