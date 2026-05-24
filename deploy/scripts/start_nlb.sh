#!/usr/bin/env bash
# Run on the NLB instance.  Installs HAProxy, writes config from config.yaml, starts it.
set -euo pipefail

APP_DIR=/opt/utterdb

dnf install -y -q haproxy python3 python3-pip
python3 -m pip install -q --upgrade pip
python3 -m pip install -q --break-system-packages pyyaml

cd "$APP_DIR"
python3 src/nlb/nlb.py

echo "NLB (HAProxy) started"
