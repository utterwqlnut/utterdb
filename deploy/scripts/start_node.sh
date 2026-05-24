#!/usr/bin/env bash
# Run on a data node instance.
# Usage: start_node.sh <private_ip> <port>
#   private_ip  — this instance's private IP (passed to the gRPC server so peers can reach it)
#   port        — TCP port to listen on
set -euo pipefail

PRIVATE_IP=${1:?private_ip required}
PORT=${2:-8000}
APP_DIR=/opt/utterdb

export PATH=$PATH:/usr/local/go/bin:/root/go/bin

cd "$APP_DIR"

# Build
go build -o bin/node ./src/node/main/main.go

cat > /etc/systemd/system/utterdb-node.service <<EOF
[Unit]
Description=utterdb data node
After=network.target

[Service]
WorkingDirectory=$APP_DIR
ExecStart=$APP_DIR/bin/node :${PORT} ${PRIVATE_IP}:${PORT}
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now utterdb-node
echo "Data node started on :${PORT} (advertised as ${PRIVATE_IP}:${PORT})"
