#!/usr/bin/env python3
import ipaddress
import os
import sys

try:
    import yaml
except ModuleNotFoundError:
    yaml = None

# Always resolve config.yaml relative to the repo root (two levels up from src/nlb/)
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
REPO_ROOT   = os.path.abspath(os.path.join(SCRIPT_DIR, "..", ".."))
config_file = os.path.join(REPO_ROOT, "config.yaml")

def load_config(path):
    if yaml is not None:
        with open(path, "r") as f:
            return yaml.safe_load(f) or {}

    data = {"proxies": [], "nlb_port": 9000}
    section = None
    with open(path, "r") as f:
        for raw_line in f:
            stripped = raw_line.strip()
            if not stripped or stripped.startswith("#"):
                continue
            if not raw_line.startswith((" ", "\t")) and stripped.endswith(":"):
                section = stripped[:-1]
                continue
            if not raw_line.startswith((" ", "\t")):
                section = None
            if section == "proxies" and stripped.startswith("- "):
                data["proxies"].append(stripped[2:].strip().strip("\"'"))
            elif stripped.startswith("nlb_port:"):
                data["nlb_port"] = stripped.split(":", 1)[1].strip()
    return data

def parse_backend(addr):
    ip, port = addr.rsplit(":", 1)
    ipaddress.ip_address(ip)
    port_num = int(port)
    if not 1 <= port_num <= 65535:
        raise ValueError(f"invalid port {port_num}")
    return ip, port_num

data = load_config(config_file)
backends = data.get("proxies", [])
if not backends:
    sys.exit("ERROR: no proxies defined in config.yaml")

nlb_port = data.get("nlb_port", 9000)
try:
    nlb_port = int(nlb_port)
    if not 1 <= nlb_port <= 65535:
        raise ValueError(f"invalid nlb_port {nlb_port}")
    parsed_backends = [parse_backend(addr) for addr in backends]
except ValueError as exc:
    sys.exit(f"ERROR: invalid NLB configuration in config.yaml: {exc}")

print("AWS Network Load Balancer is managed by Terraform.")
print(f"Listener port: {nlb_port}")
print("Proxy targets:")
for index, (ip, port) in enumerate(parsed_backends):
    print(f"  proxy-{index}: {ip}:{port}")
