#!/usr/bin/env bash
# utterdb AWS deployment orchestrator
#
# Prerequisites:
#   - terraform, aws CLI, jq installed locally
#   - SSH key pair; agent loaded or pass --key explicitly
#   - AWS credentials in environment or active SSO session
#
# Usage:
#   ./deploy/deploy.sh [--key ~/.ssh/id_rsa] [--nodes 3] [--proxies 2] [--benchmark-nodes 3] [--type t3.small]
#
# Flow:
#   1. terraform apply  — provision instances + AWS NLB
#   2. Generate config.yaml from real private IPs
#   3. Wait for SSH on every instance
#   4. Bootstrap every instance: git clone + write config.yaml
#   5. Start services in order: nodes → manipulator → proxies
#   6. Start Locust on benchmark nodes against the NLB DNS name
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
TF_DIR="$SCRIPT_DIR/terraform"

# ── Defaults ──────────────────────────────────────────────────────────────────
SSH_KEY="${HOME}/.ssh/id_ed25519"
NODE_COUNT=2
PROXY_COUNT=1
INSTANCE_TYPE="t3.micro"             # nodes + proxies + manipulator
BENCHMARK_INSTANCE_TYPE="m7a.xlarge" # benchmark runner
BENCHMARK_COUNT=1
BENCH_USERS=50
BENCH_SPAWN_RATE=10
BENCH_DURATION=60s
SSH_USER="ec2-user"
NODE_BASE_PORT=8000
PROXY_BASE_PORT=8100
MANIPULATOR_PORT=8080
NLB_PORT=9000

# ── Argument parsing ──────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case $1 in
    --key)             SSH_KEY="$2";                  shift 2 ;;
    --nodes)           NODE_COUNT="$2";               shift 2 ;;
    --proxies)         PROXY_COUNT="$2";              shift 2 ;;
    --type)            INSTANCE_TYPE="$2";            shift 2 ;;
    --nlb-type)        echo "WARN: --nlb-type is ignored; AWS NLB is managed by AWS"; shift 2 ;;
    --benchmark-type)  BENCHMARK_INSTANCE_TYPE="$2";  shift 2 ;;
    --benchmark-nodes|--benchmarks) BENCHMARK_COUNT="$2"; shift 2 ;;
    --users|--workers) BENCH_USERS="$2";              shift 2 ;;
    --spawn-rate)      BENCH_SPAWN_RATE="$2";         shift 2 ;;
    --duration)        BENCH_DURATION="$2";           shift 2 ;;
    --seed)            echo "WARN: --seed is ignored by the Locust benchmark"; shift 2 ;;
    *) echo "Unknown option: $1"; exit 1 ;;
  esac
done

PUBLIC_KEY_PATH="${SSH_KEY}.pub"
if [[ ! -f "$PUBLIC_KEY_PATH" ]]; then
  echo "ERROR: public key not found at $PUBLIC_KEY_PATH"
  exit 1
fi

# ── Helpers ───────────────────────────────────────────────────────────────────
wait_for_ssh() {
  local ip=$1
  echo -n "  Waiting for SSH on $ip ..."
  for i in $(seq 1 40); do
    if ssh -o StrictHostKeyChecking=no -o ConnectTimeout=5 \
           -i "$SSH_KEY" "${SSH_USER}@${ip}" true 2>/dev/null; then
      echo " ready"
      return 0
    fi
    sleep 6
    echo -n "."
  done
  echo " TIMEOUT"
  return 1
}

remote() {
  local ip=$1; shift
  ssh -o StrictHostKeyChecking=no -i "$SSH_KEY" "${SSH_USER}@${ip}" "$@"
}

# ── 1. Terraform apply ────────────────────────────────────────────────────────
echo "==> Provisioning infrastructure with Terraform..."
cd "$TF_DIR"
terraform init -input=false -upgrade -reconfigure
terraform apply -auto-approve \
  -var="node_count=${NODE_COUNT}" \
  -var="proxy_count=${PROXY_COUNT}" \
  -var="instance_type=${INSTANCE_TYPE}" \
  -var="benchmark_count=${BENCHMARK_COUNT}" \
  -var="benchmark_instance_type=${BENCHMARK_INSTANCE_TYPE}" \
  -var="public_key_path=${PUBLIC_KEY_PATH}"

# ── 2. Collect IPs ────────────────────────────────────────────────────────────
echo "==> Reading Terraform outputs..."
MANIPULATOR_PUB=$(terraform output -raw manipulator_public_ip)
MANIPULATOR_PRIV=$(terraform output -raw manipulator_private_ip)

NODE_PUB_IPS=($(terraform output -json node_public_ips  | jq -r '.[]'))
NODE_PRIV_IPS=($(terraform output -json node_private_ips | jq -r '.[]'))

PROXY_PUB_IPS=($(terraform output -json proxy_public_ips  | jq -r '.[]'))
PROXY_PRIV_IPS=($(terraform output -json proxy_private_ips | jq -r '.[]'))

NLB_DNS=$(terraform output -raw nlb_dns_name)

BENCHMARK_PUB_IPS=($(terraform output -json benchmark_public_ips | jq -r '.[]'))
BENCHMARK_PRIV_IPS=($(terraform output -json benchmark_private_ips | jq -r '.[]'))
BENCHMARK_MASTER_PUB="${BENCHMARK_PUB_IPS[0]}"
BENCHMARK_MASTER_PRIV="${BENCHMARK_PRIV_IPS[0]}"

cd "$REPO_ROOT"

# ── 3. Generate config.yaml from real private IPs ─────────────────────────────
echo "==> Generating config.yaml..."

NODES_YAML=""
for i in "${!NODE_PRIV_IPS[@]}"; do
  PORT=$(( NODE_BASE_PORT + i ))
  NODES_YAML+="  - ${NODE_PRIV_IPS[$i]}:${PORT}"$'\n'
done

PROXIES_YAML=""
for i in "${!PROXY_PRIV_IPS[@]}"; do
  PORT=$(( PROXY_BASE_PORT + i ))
  PROXIES_YAML+="  - ${PROXY_PRIV_IPS[$i]}:${PORT}"$'\n'
done

CONFIG_YAML="nodes:
${NODES_YAML}proxies:
${PROXIES_YAML}manipulator: ${MANIPULATOR_PRIV}:${MANIPULATOR_PORT}
nlb_port: ${NLB_PORT}
replication_factor: 2
shards: 128
memory:
  swappiness: 0
"

echo "Generated config.yaml:"
echo "$CONFIG_YAML"

# Update local config.yaml so it reflects the deployed cluster
echo "$CONFIG_YAML" > "$REPO_ROOT/config.yaml"
echo "  Local config.yaml updated."

# Encode for safe remote transfer (avoids quoting/newline issues over SSH)
CONFIG_B64=$(echo "$CONFIG_YAML" | base64)

# ── 4. Wait for SSH ───────────────────────────────────────────────────────────
echo "==> Waiting for instances to become reachable..."
ALL_PUB_IPS=("$MANIPULATOR_PUB" "${NODE_PUB_IPS[@]}" "${PROXY_PUB_IPS[@]}" "${BENCHMARK_PUB_IPS[@]}")
for ip in "${ALL_PUB_IPS[@]}"; do
  wait_for_ssh "$ip"
done

# ── 5. Bootstrap all instances in parallel ────────────────────────────────────
echo "==> Bootstrapping all instances (git clone + config.yaml)..."
bootstrap_instance() {
  local ip=$1 config_b64=$2
  echo "  Bootstrapping $ip..."
  remote "$ip" "sudo bash -s -- '${config_b64}'" \
    < "$SCRIPT_DIR/scripts/bootstrap_common.sh"
}

for ip in "${ALL_PUB_IPS[@]}"; do
  bootstrap_instance "$ip" "$CONFIG_B64" &
done
wait
echo "  All instances bootstrapped."

# ── 6. Start data nodes ───────────────────────────────────────────────────────
echo "==> Starting data nodes..."
for i in "${!NODE_PUB_IPS[@]}"; do
  ip="${NODE_PUB_IPS[$i]}"
  priv="${NODE_PRIV_IPS[$i]}"
  port=$(( NODE_BASE_PORT + i ))
  echo "  node-${i}: ${ip} -> :${port}"
  remote "$ip" "sudo bash /opt/utterdb/deploy/scripts/start_node.sh ${priv} ${port}" &
done
wait
echo "  Data nodes started."

# ── 7. Start manipulator ──────────────────────────────────────────────────────
echo "==> Starting manipulator (${MANIPULATOR_PUB}:${MANIPULATOR_PORT})..."
remote "$MANIPULATOR_PUB" "sudo bash /opt/utterdb/deploy/scripts/start_manipulator.sh ${MANIPULATOR_PORT}"

# ── 8. Start proxies ──────────────────────────────────────────────────────────
echo "==> Starting proxies..."
for i in "${!PROXY_PUB_IPS[@]}"; do
  ip="${PROXY_PUB_IPS[$i]}"
  port=$(( PROXY_BASE_PORT + i ))
  echo "  proxy-${i}: ${ip} -> :${port}"
  remote "$ip" "sudo bash /opt/utterdb/deploy/scripts/start_proxy.sh ${port}" &
done
wait
echo "  Proxies started."

# ── 9. Start distributed Locust benchmark ─────────────────────────────────────
echo "==> Installing Locust on benchmark nodes..."
for ip in "${BENCHMARK_PUB_IPS[@]}"; do
  remote "$ip" "sudo bash /opt/utterdb/deploy/scripts/start_benchmark.sh install" &
done
wait

echo "==> Starting Locust benchmark (${BENCH_USERS} users, ${BENCH_SPAWN_RATE}/s, ${BENCH_DURATION})..."
if [[ "${#BENCHMARK_PUB_IPS[@]}" -eq 1 ]]; then
  remote "$BENCHMARK_MASTER_PUB" \
    "sudo nohup bash /opt/utterdb/deploy/scripts/start_benchmark.sh local '${NLB_DNS}' '${NLB_PORT}' '${BENCH_USERS}' '${BENCH_SPAWN_RATE}' '${BENCH_DURATION}' > /tmp/utterdb-locust.log 2>&1 < /dev/null &"
  echo "  local Locust run: ${BENCHMARK_MASTER_PUB}:/tmp/utterdb-locust.log"
else
  LOCUST_WORKERS=$(( ${#BENCHMARK_PUB_IPS[@]} - 1 ))
  remote "$BENCHMARK_MASTER_PUB" \
    "sudo nohup bash /opt/utterdb/deploy/scripts/start_benchmark.sh master '${NLB_DNS}' '${NLB_PORT}' '${BENCH_USERS}' '${BENCH_SPAWN_RATE}' '${BENCH_DURATION}' '${BENCHMARK_MASTER_PRIV}' '${LOCUST_WORKERS}' > /tmp/utterdb-locust-master.log 2>&1 < /dev/null &"
  echo "  master: ${BENCHMARK_MASTER_PUB}:/tmp/utterdb-locust-master.log"
  sleep 5
  for i in "${!BENCHMARK_PUB_IPS[@]}"; do
    if [[ "$i" -eq 0 ]]; then
      continue
    fi
    remote "${BENCHMARK_PUB_IPS[$i]}" \
      "sudo nohup bash /opt/utterdb/deploy/scripts/start_benchmark.sh worker '${NLB_DNS}' '${NLB_PORT}' '${BENCH_USERS}' '${BENCH_SPAWN_RATE}' '${BENCH_DURATION}' '${BENCHMARK_MASTER_PRIV}' > /tmp/utterdb-locust-worker.log 2>&1 < /dev/null &"
    echo "  worker-${i}: ${BENCHMARK_PUB_IPS[$i]}:/tmp/utterdb-locust-worker.log"
  done
fi

# ── 10. Summary ───────────────────────────────────────────────────────────────
echo ""
echo "========================================="
echo " utterdb cluster is up"
echo "========================================="
printf " Manipulator : %s:%s\n" "$MANIPULATOR_PUB" "$MANIPULATOR_PORT"
for i in "${!NODE_PUB_IPS[@]}"; do
  printf " Node %-2s      : %s:%s\n" "$i" "${NODE_PUB_IPS[$i]}" "$(( NODE_BASE_PORT + i ))"
done
for i in "${!PROXY_PUB_IPS[@]}"; do
  printf " Proxy %-2s     : %s:%s\n" "$i" "${PROXY_PUB_IPS[$i]}" "$(( PROXY_BASE_PORT + i ))"
done
printf " NLB          : %s:%s\n" "$NLB_DNS" "$NLB_PORT"
printf " Locust master: %s\n" "$BENCHMARK_MASTER_PUB"
for i in "${!BENCHMARK_PUB_IPS[@]}"; do
  if [[ "$i" -eq 0 ]]; then
    continue
  fi
  printf " Locust worker %-2s: %s\n" "$i" "${BENCHMARK_PUB_IPS[$i]}"
done
echo ""
echo "Locust logs:"
if [[ "${#BENCHMARK_PUB_IPS[@]}" -eq 1 ]]; then
  echo "  ssh -i $SSH_KEY ${SSH_USER}@${BENCHMARK_MASTER_PUB} 'sudo tail -f /tmp/utterdb-locust.log'"
else
  echo "  ssh -i $SSH_KEY ${SSH_USER}@${BENCHMARK_MASTER_PUB} 'sudo tail -f /tmp/utterdb-locust-master.log'"
fi
echo "========================================="
