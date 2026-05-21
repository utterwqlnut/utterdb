import os

import yaml

with open("config.yaml", "r") as f:
    data = yaml.safe_load(f)

for ip in data["proxies"]:
    os.system("ipvsadm -A -t localhost:8080 -r " + ip + " -m")
