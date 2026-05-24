#!/usr/bin/env bash
# Run on a proxy instance.
# Usage: start_proxy.sh <port>
set -euo pipefail

PORT=${1:-8100}
APP_DIR=/opt/utterdb

export PATH=$PATH:/usr/local/go/bin:/root/go/bin

cd "$APP_DIR"

# Build
go build -o bin/proxy ./src/proxy/main.go

cat > /etc/systemd/system/utterdb-proxy.service <<EOF
[Unit]
Description=utterdb proxy
After=network.target

[Service]
WorkingDirectory=$APP_DIR
ExecStart=$APP_DIR/bin/proxy :${PORT}
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now utterdb-proxy
echo "Proxy started on :${PORT}"
