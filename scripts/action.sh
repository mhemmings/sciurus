#!/usr/bin/env bash
set -euo pipefail

cd "$GITHUB_WORKSPACE"

case "$SCIURUS_COMMAND" in
  validate)
    if [[ -z "$SCIURUS_MODEL" ]]; then
      echo '::error::The model input is required for validation.' >&2
      exit 2
    fi
    sciurus validate -- "$SCIURUS_MODEL"
    ;;
  diff)
    if [[ -z "$SCIURUS_BEFORE" || -z "$SCIURUS_AFTER" ]]; then
      echo '::error::The before and after inputs are required for a diff.' >&2
      exit 2
    fi
    case "$SCIURUS_FORMAT" in
      text) extension=txt ;;
      png) extension=png ;;
      *) echo '::error::The format input must be text or png.' >&2; exit 2 ;;
    esac
    case "$SCIURUS_FAIL_ON_CHANGES" in
      true|false) ;;
      *) echo '::error::The fail-on-changes input must be true or false.' >&2; exit 2 ;;
    esac

    if [[ -n "$SCIURUS_OUTPUT" ]]; then
      report=$SCIURUS_OUTPUT
      if [[ "$report" != /* ]]; then
        report="$GITHUB_WORKSPACE/$report"
      fi
      mkdir -p "$(dirname "$report")"
    else
      report_directory=$(mktemp -d "$RUNNER_TEMP/sciurus-diff.XXXXXX")
      report="$report_directory/schema-diff.$extension"
    fi

    if sciurus diff --exit-code --format "$SCIURUS_FORMAT" --output "$report" -- "$SCIURUS_BEFORE" "$SCIURUS_AFTER"; then
      changed=false
    else
      status=$?
      if [[ "$status" != 1 ]]; then
        exit "$status"
      fi
      changed=true
    fi

    # Reports and outputs remain available when schema differences fail a check.
    delimiter="sciurus_report_${RANDOM}_${RANDOM}"
    while [[ "$report" == *"$delimiter"* ]]; do
      delimiter="sciurus_report_${RANDOM}_${RANDOM}"
    done
    {
      printf 'changed=%s\n' "$changed"
      printf 'report<<%s\n%s\n%s\n' "$delimiter" "$report" "$delimiter"
    } >> "$GITHUB_OUTPUT"
    if [[ "$SCIURUS_FORMAT" == text ]]; then
      cat "$report"
    else
      printf 'PNG schema diff: %s\n' "$report"
    fi

    if [[ "$changed" == true && "$SCIURUS_FAIL_ON_CHANGES" == true ]]; then
      echo '::error::The schemas contain database differences.' >&2
      exit 1
    fi
    ;;
  *)
    echo '::error::The command input must be validate or diff.' >&2
    exit 2
    ;;
esac
