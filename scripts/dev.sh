#!/usr/bin/env bash
# Run both development servers and stop the Go process when Vite exits.
set -euo pipefail
cd "$(dirname "$0")/.."
server_log=$(mktemp)
./bin/sciurus serve --addr 127.0.0.1:7480 >"$server_log" 2>&1 &
server_pid=$!
trap 'kill "$server_pid" 2>/dev/null || true; wait "$server_pid" 2>/dev/null || true; rm -f "$server_log"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
# Wait for this process to confirm that it owns the listener. An HTTP readiness
# probe alone could accidentally find another Sciurus instance on the same port.
until IFS= read -r server_line <"$server_log" && [[ "$server_line" == 'Sciurus → http://'* ]]; do
  if ! kill -0 "$server_pid" 2>/dev/null; then
    cat "$server_log" >&2
    wait "$server_pid"
    exit 1
  fi
  sleep 0.1
done
cat "$server_log"
cd web
yarn dev --strictPort
