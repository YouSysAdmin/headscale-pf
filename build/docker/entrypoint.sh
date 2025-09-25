#!/bin/sh
# shellcheck shell=sh
set -eu
(set -o pipefail) 2>/dev/null || true

log()  { printf '%s %s\n' "$(date -Iseconds)" "$*"; }
err()  { printf '%s ERROR: %s\n' "$(date -Iseconds)" "$*" >&2; }
fail() { err "$*"; exit 1; }

# Config
: "${PF_TOKEN:?PF_TOKEN is required (e.g., jca_xxx)}"

WORK_DIR="${WORK_DIR:-/work}"
INPUT_POLICY="${INPUT_POLICY:-${WORK_DIR}/policy.hjson}"
OUTPUT_POLICY="${OUTPUT_POLICY:-${WORK_DIR}/policy.json}"
SOURCE="${SOURCE:-jc}"

RETRIES="${RETRIES:-3}"
RETRY_DELAY_SEC="${RETRY_DELAY_SEC:-5}"

mkdir -p "$WORK_DIR"

# Prepare policy
log "Running headscale-pf prepare (source=${SOURCE}) input=${INPUT_POLICY} output=${OUTPUT_POLICY}"
if ! sh -c "headscale-pf --input-policy '${INPUT_POLICY}' --output-policy '${OUTPUT_POLICY}' prepare --source '${SOURCE}' ${HEADSCALE_PF_EXTRA_FLAGS:-}"; then
  rc=$?
  fail "headscale-pf prepare failed with exit code ${rc}"
fi
log "headscale-pf prepare: OK"

# Apply policy
if [ -n "${APPLY_POLICY:-}" ]; then
  [ -n "${HEADSCALE_CLI_ADDRESS:-}" ] || fail "APPLY_POLICY=1 requires HEADSCALE_CLI_ADDRESS"
  [ -n "${HEADSCALE_CLI_API_KEY:-}" ] || fail "APPLY_POLICY=1 requires HEADSCALE_CLI_API_KEY"

  log "Applying policy via headscale remote CLI to ${HEADSCALE_CLI_ADDRESS}"

  i=1
  while :; do
    if headscale policy set -f "${OUTPUT_POLICY}"; then
      log "Policy applied."
      break
    fi
    rc=$?
    if [ "$i" -ge "$RETRIES" ]; then
      fail "headscale policy set failed after ${i} attempt(s), exit code ${rc}"
    fi
    log "headscale policy set failed (attempt ${i}/${RETRIES}, rc=${rc}); retrying in ${RETRY_DELAY_SEC}s..."
    i=$((i+1))
    sleep "$RETRY_DELAY_SEC"
  done
else
  log "APPLY_POLICY not set — skipping headscale policy set."
fi

log "Done."
