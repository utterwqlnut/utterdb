#!/usr/bin/env python3
import os
import shutil
import subprocess
import sys

import yaml

CONFIG_PATH = "/etc/haproxy/haproxy.cfg"

# Always resolve config.yaml relative to the repo root (two levels up from src/nlb/)
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
REPO_ROOT   = os.path.abspath(os.path.join(SCRIPT_DIR, "..", ".."))
config_file = os.path.join(REPO_ROOT, "config.yaml")

# ── Sanity checks ─────────────────────────────────────────────────────────────
if not shutil.which("haproxy"):
    sys.exit("ERROR: haproxy binary not found. Install it first (e.g. dnf install haproxy).")

with open(config_file, "r") as f:
    data = yaml.safe_load(f)

backends = data.get("proxies", [])
if not backends:
    sys.exit("ERROR: no proxies defined in config.yaml")

nlb_port = data.get("nlb_port", 9000)

# ── Backend lines ─────────────────────────────────────────────────────────────
# Correct HAProxy syntax: server <name> <ip>:<port>
def format_backend(addr, index):
    ip, port = addr.rsplit(":", 1)
    safe_name = f"proxy_{index}_{ip.replace('.', '_')}"
    return f"    server {safe_name} {ip}:{port}"

backend_block = "\n".join(format_backend(addr, i) for i, addr in enumerate(backends))

# ── Config ────────────────────────────────────────────────────────────────────
haproxy_conf = f"""
global
    daemon
    maxconn 4096

defaults
    mode tcp
    timeout connect 5s
    timeout client  1m
    timeout server  1m
    timeout tunnel  1h

frontend tcp_in
    bind *:{nlb_port}
    default_backend tcp_backends

backend tcp_backends
    balance roundrobin
{backend_block}
"""

os.makedirs("/etc/haproxy", exist_ok=True)

with open(CONFIG_PATH, "w") as f:
    f.write(haproxy_conf)

# ── Validate ──────────────────────────────────────────────────────────────────
result = subprocess.run(
    ["haproxy", "-c", "-f", CONFIG_PATH],
    capture_output=True, text=True
)
if result.returncode != 0:
    print("HAProxy config validation FAILED:")
    print(result.stderr)
    sys.exit(1)

# ── Start ─────────────────────────────────────────────────────────────────────
subprocess.run(["systemctl", "enable", "haproxy"], check=True)
subprocess.run(["systemctl", "restart", "haproxy"], check=True)

print(f"HAProxy TCP load balancer started on :{nlb_port} -> {backends}")
