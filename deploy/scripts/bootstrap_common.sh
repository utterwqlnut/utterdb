#!/usr/bin/env bash
# Installed on every instance before role-specific setup.
# Usage: bootstrap_common.sh <base64-encoded-config-yaml>
set -euo pipefail

CONFIG_B64=${1:?base64-encoded config.yaml required}

# ── System packages ───────────────────────────────────────────────────────────
dnf update -y -q
dnf install -y -q golang git

# ── Go environment ────────────────────────────────────────────────────────────
export GOPATH=/root/go
export PATH=$PATH:/usr/local/go/bin:$GOPATH/bin
echo 'export GOPATH=/root/go'                        >> /etc/profile.d/go.sh
echo 'export PATH=$PATH:/usr/local/go/bin:$GOPATH/bin' >> /etc/profile.d/go.sh

# ── Clone repo ────────────────────────────────────────────────────────────────
APP_DIR=/opt/utterdb

# Allow root to operate on this directory regardless of who owns it
git config --global --add safe.directory "$APP_DIR"

if [[ -d "$APP_DIR/.git" ]]; then
  echo "Repo already cloned, pulling latest..."
  git -C "$APP_DIR" pull --ff-only
else
  git clone https://github.com/utterwqlnut/utterdb.git "$APP_DIR"
fi

# Ensure root owns everything so subsequent sudo commands work cleanly
chown -R root:root "$APP_DIR"

# ── Write config.yaml from the generated content passed in ───────────────────
echo "$CONFIG_B64" | base64 -d > "$APP_DIR/config.yaml"
echo "config.yaml written:"
cat "$APP_DIR/config.yaml"
