#!/usr/bin/env bash
# Test schema equivalence using disposable databases and optional local Docker.
set -euo pipefail
cd "$(dirname "$0")"
export REQUIRE_MYSQL="${REQUIRE_MYSQL:-1}"

if [[ -z "${DB_ADDR:-}" ]]; then
  command -v docker >/dev/null 2>&1 || { echo 'Docker is required when DB_ADDR is unset.' >&2; exit 1; }
  container_id=$(docker run --rm -d \
    -e MYSQL_ROOT_PASSWORD=sciurus-test \
    -p 127.0.0.1::3306 \
    --tmpfs /var/lib/mysql \
    mysql:8.4.9)
  trap 'docker rm -f "$container_id" >/dev/null 2>&1 || true' EXIT
  trap 'exit 130' INT
  trap 'exit 143' TERM
  export DB_ADDR
  DB_ADDR=$(docker port "$container_id" 3306/tcp)
  export DB_USER=root DB_PASS=sciurus-test
  echo "Waiting for disposable MySQL at $DB_ADDR..."
  ready=false
  for attempt in {1..90}; do
    if docker exec "$container_id" mysqladmin ping -h127.0.0.1 -uroot -psciurus-test --silent >/dev/null 2>&1; then
      ready=true
      break
    fi
    sleep 1
  done
  if [[ "$ready" != true ]]; then
    echo 'MySQL did not become ready within 90 seconds.' >&2
    docker logs "$container_id" >&2
    exit 1
  fi
fi

# Arguments are Go package patterns; the default covers the complete module.
if [[ $# -eq 0 ]]; then set -- ./...; fi
go vet "$@"
go test -race -count=1 "$@"
