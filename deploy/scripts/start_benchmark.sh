#!/usr/bin/env bash
# Run on benchmark instances.
# Usage:
#   start_benchmark.sh install
#   start_benchmark.sh local  <nlb_endpoint> <nlb_port> [users] [spawn_rate] [duration]
#   start_benchmark.sh master <nlb_endpoint> <nlb_port> [users] [spawn_rate] [duration] [master_host] [expect_workers]
#   start_benchmark.sh worker <nlb_endpoint> <nlb_port> [users] [spawn_rate] [duration] <master_host>
set -euo pipefail

APP_DIR=/opt/utterdb
PYTHON_BIN=${PYTHON_BIN:-python3}

ROLE=${1:-local}
case "$ROLE" in
  install|local|master|worker)
    shift
    ;;
  *)
    # Backwards compatibility: old usage started with the endpoint.
    ROLE=local
    ;;
esac

install_locust() {
  if command -v dnf >/dev/null 2>&1; then
    dnf install -y -q python3 python3-pip python3-devel gcc make python2-devel \
      || dnf install -y -q python3 python3-pip python3-devel gcc make
  fi

  "$PYTHON_BIN" -m pip install -q --upgrade pip setuptools wheel
  "$PYTHON_BIN" -m pip install -q --ignore-installed locust
  "$PYTHON_BIN" -m locust --version >/dev/null
}

if [[ "$ROLE" == "install" ]]; then
  install_locust
  echo "Locust installed for $("$PYTHON_BIN" --version 2>&1)"
  exit 0
fi

NLB_ENDPOINT=${1:?nlb_endpoint required}
NLB_PORT=${2:-9000}
USERS=${3:-50}
SPAWN_RATE=${4:-10}
DURATION=${5:-60s}
MASTER_HOST=${6:-127.0.0.1}
EXPECT_WORKERS=${7:-1}

install_locust

# Avoid stacking stale benchmark runs when redeploying.
pkill -f "locust.*src/test/benchmark.py" 2>/dev/null || true

LOCUST_CMD=(
  "$PYTHON_BIN" -m locust
  -f src/test/benchmark.py
  --host "tcp://${NLB_ENDPOINT}:${NLB_PORT}"
)

echo "========================================="
echo " utterdb benchmark (Locust ${ROLE})"
echo " target   : ${NLB_ENDPOINT}:${NLB_PORT}"
echo " users    : ${USERS}  spawn_rate: ${SPAWN_RATE}/s  duration: ${DURATION}"
if [[ "$ROLE" == "master" ]]; then
  echo " workers  : expecting ${EXPECT_WORKERS}"
elif [[ "$ROLE" == "worker" ]]; then
  echo " master   : ${MASTER_HOST}"
fi
echo "========================================="

cd "$APP_DIR"

case "$ROLE" in
  local)
    exec "${LOCUST_CMD[@]}" \
      --headless \
      -u "$USERS" \
      -r "$SPAWN_RATE" \
      --run-time "$DURATION"
    ;;
  master)
    exec "${LOCUST_CMD[@]}" \
      --headless \
      --master \
      --master-bind-host 0.0.0.0 \
      --expect-workers "$EXPECT_WORKERS" \
      -u "$USERS" \
      -r "$SPAWN_RATE" \
      --run-time "$DURATION"
    ;;
  worker)
    exec "${LOCUST_CMD[@]}" \
      --worker \
      --master-host "$MASTER_HOST"
    ;;
esac
