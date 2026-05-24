import os
import subprocess
import sys

import yaml

CONFIG_PATH = "/etc/haproxy/haproxy.cfg"

# Always load config.yaml relative to the repo root (two levels up from src/nlb/)
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
REPO_ROOT = os.path.join(SCRIPT_DIR, "..", "..")
config_file = os.path.join(REPO_ROOT, "config.yaml")

with open(config_file, "r") as f:
    data = yaml.safe_load(f)

backends = data["proxies"]
nlb_port = data.get("nlb_port", 9000)


# FIX: split IP:PORT safely for HAProxy
def format_backend(addr):
    ip, port = addr.split(":")
    return f"server {ip.replace('.', '_')} {ip} port {port}"


backend_block = "\n".join([f"    {format_backend(addr)}" for addr in backends])

haproxy_conf = f"""
global
    daemon
    maxconn 4096
    log stdout format raw local0

defaults
    mode tcp
    timeout connect 5s
    timeout client  1m
    timeout server  1m
    timeout tunnel  1h
    option clitcpka
    option srvtcpka

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

# validate
subprocess.run(["haproxy", "-c", "-f", CONFIG_PATH], check=True)

# enable + restart (works whether haproxy was previously running or not)
subprocess.run(["systemctl", "enable", "haproxy"], check=True)
subprocess.run(["systemctl", "restart", "haproxy"], check=True)

print("HAProxy TCP load balancer updated successfully")
