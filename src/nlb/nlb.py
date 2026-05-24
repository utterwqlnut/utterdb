import os
import subprocess

import yaml

CONFIG_PATH = "/etc/haproxy/haproxy.cfg"

with open("config.yaml", "r") as f:
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

defaults
    mode tcp
    timeout connect 5s
    timeout client  1m
    timeout server  1m

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
