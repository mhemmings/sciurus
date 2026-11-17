#!/usr/bin/env bash
set -euo pipefail

project_directory=$(cd "$(dirname "$0")/.." && pwd)
test_directory=$(mktemp -d)
trap 'rm -rf "$test_directory"' EXIT
mkdir -p "$test_directory/bin" "$test_directory/workspace with spaces" "$test_directory/runner"
(
  cd "$project_directory"
  GOWORK=off GOBIN="$test_directory/bin" go install ./cmd/sciurus
)
export PATH="$test_directory/bin:$PATH"
export GITHUB_WORKSPACE="$test_directory/workspace with spaces"
export RUNNER_TEMP="$test_directory/runner"
export GITHUB_OUTPUT="$test_directory/outputs"

cd "$GITHUB_WORKSPACE"
cat > before.sql <<'SQL'
CREATE TABLE task (id INT NOT NULL, PRIMARY KEY (id));
SQL
cat > after.sql <<'SQL'
CREATE TABLE task (id INT NOT NULL, title VARCHAR(80) NULL, PRIMARY KEY (id));
SQL
sciurus import --output 'model with spaces.er.json' before.sql >/dev/null
printf '{"version":999}\n' > invalid.json
printf 'SELECT 1;\n' > invalid.sql

run_action() {
  local expected=$1
  shift
  : > "$GITHUB_OUTPUT"
  local status=0
  env SCIURUS_COMMAND=validate SCIURUS_MODEL= SCIURUS_BEFORE= SCIURUS_AFTER= \
    SCIURUS_FORMAT=text SCIURUS_OUTPUT= SCIURUS_FAIL_ON_CHANGES=false \
    "$@" bash "$project_directory/scripts/action.sh" > "$test_directory/log" 2>&1 || status=$?
  if [[ "$status" != "$expected" ]]; then
    cat "$test_directory/log" >&2
    printf 'Expected exit %s, received %s\n' "$expected" "$status" >&2
    exit 1
  fi
}

run_action 0 'SCIURUS_MODEL=model with spaces.er.json'
run_action 1 SCIURUS_MODEL=invalid.json
run_action 2

run_action 0 SCIURUS_COMMAND=diff 'SCIURUS_BEFORE=model with spaces.er.json' \
  SCIURUS_AFTER=before.sql SCIURUS_FAIL_ON_CHANGES=true
grep -qx 'changed=false' "$GITHUB_OUTPUT"
report=$(sed -n '3p' "$GITHUB_OUTPUT")
grep -qx 'No database changes.' "$report"

run_action 0 SCIURUS_COMMAND=diff SCIURUS_BEFORE=before.sql SCIURUS_AFTER=after.sql
grep -qx 'changed=true' "$GITHUB_OUTPUT"
report=$(sed -n '3p' "$GITHUB_OUTPUT")
grep -q 'title' "$report"

run_action 1 SCIURUS_COMMAND=diff SCIURUS_BEFORE=before.sql SCIURUS_AFTER=after.sql \
  SCIURUS_FAIL_ON_CHANGES=true 'SCIURUS_OUTPUT=reports with spaces/schema diff.txt'
grep -qx 'changed=true' "$GITHUB_OUTPUT"
report=$(sed -n '3p' "$GITHUB_OUTPUT")
[[ "$report" == "$GITHUB_WORKSPACE/reports with spaces/schema diff.txt" ]]
grep -q 'title' "$report"

run_action 1 SCIURUS_COMMAND=diff SCIURUS_BEFORE=before.sql SCIURUS_AFTER=after.sql \
  SCIURUS_FORMAT=png SCIURUS_FAIL_ON_CHANGES=true
grep -qx 'changed=true' "$GITHUB_OUTPUT"
report=$(sed -n '3p' "$GITHUB_OUTPUT")
[[ "$(od -An -tx1 -N8 "$report" | tr -d ' \n')" == 89504e470d0a1a0a ]]
grep -q '^PNG schema diff:' "$test_directory/log"

run_action 2 SCIURUS_COMMAND=diff SCIURUS_BEFORE=before.sql SCIURUS_AFTER=invalid.sql
[[ ! -s "$GITHUB_OUTPUT" ]]

# File names are CLI operands even when they look like flags or shell expressions.
model='--diagram $(touch injected).er.json'
before='-before $(touch injected).sql'
after='--after $(touch injected).sql'
cp 'model with spaces.er.json' "./$model"
cp before.sql "./$before"
cp after.sql "./$after"
run_action 0 "SCIURUS_MODEL=$model"
run_action 0 SCIURUS_COMMAND=diff "SCIURUS_BEFORE=$before" "SCIURUS_AFTER=$after"
grep -qx 'changed=true' "$GITHUB_OUTPUT"
[[ ! -e injected ]]

report=$'reports/schema\nreport.txt'
run_action 0 SCIURUS_COMMAND=diff SCIURUS_BEFORE=before.sql SCIURUS_AFTER=after.sql "SCIURUS_OUTPUT=$report"
delimiter=$(sed -n '2s/^report<<//p' "$GITHUB_OUTPUT")
[[ -n "$delimiter" && -s "$GITHUB_WORKSPACE/$report" ]]
printf 'changed=true\nreport<<%s\n%s\n%s\n' "$delimiter" "$GITHUB_WORKSPACE/$report" "$delimiter" > "$test_directory/expected-outputs"
cmp "$test_directory/expected-outputs" "$GITHUB_OUTPUT"

echo 'GitHub Action checks passed.'
