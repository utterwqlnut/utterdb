#!/usr/bin/env bash
# Compatibility helper. The AWS NLB itself is provisioned by Terraform.
set -euo pipefail

APP_DIR=/opt/utterdb

dnf install -y -q python3

cd "$APP_DIR"
python3 src/nlb/nlb.py

echo "AWS NLB configuration validated"
