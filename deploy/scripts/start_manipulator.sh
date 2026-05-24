#!/usr/bin/env bash
# Run on the manipulator instance.
# Usage: start_manipulator.sh <listen_port>
set -euo pipefail

PORT=${1:-8080}
APP_DIR=/opt/utterdb

export PATH=$PATH:/usr/local/go/bin:/root/go/bin

cd "$APP_DIR"

# Build
go build -o bin/manipulator ./src/manipulator/main.go

# Run as a background service via a simple systemd unit
cat > /etc/systemd/system/utterdb-manipulator.service <<EOF
[Unit]
Description=utterdb manipulator
After=network.target

[Service]
WorkingDirectory=$APP_DIR
ExecStart=$APP_DIR/bin/manipulator :${PORT}
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now utterdb-manipulator
echo "Manipulator started on :${PORT}"
