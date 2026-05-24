#!/usr/bin/env bash
# Run on a proxy instance.
# Usage: start_proxy.sh <port>
set -euo pipefail

PORT=${1:-8100}
APP_DIR=/opt/utterdb

export PATH=$PATH:/usr/local/go/bin:/root/go/bin

cd "$APP_DIR"

# Wait for all data nodes listed in config.yaml to be reachable
# before starting the proxy so gRPC connections succeed at startup
echo "Waiting for data nodes to be reachable..."
python3 - <<'PYEOF'
import yaml, socket, time, sys

with open("config.yaml") as f:
    cfg = yaml.safe_load(f)

for addr in cfg.get("nodes", []):
    host, port = addr.rsplit(":", 1)
    for attempt in range(30):
        try:
            s = socket.create_connection((host, int(port)), timeout=2)
            s.close()
            print(f"  {addr} ready")
            break
        except OSError:
            if attempt == 29:
                print(f"  {addr} TIMEOUT - proceeding anyway")
            time.sleep(2)
PYEOF

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
