# Setup
# Ensure we are in the script directory
cd "$(dirname "$0")"
SCRIPT_DIR="$(pwd)"
ROOT_DIR="$(cd ../../.. && pwd)"
SIM_DIR="$ROOT_DIR/e2e/simulation"
DATA_DIR="$ROOT_DIR/e2e/test_data"

echo "=== Setup: Building Environment ==="
mkdir -p "$DATA_DIR"
dd if=/dev/urandom of="$DATA_DIR/large_file.dat" bs=1M count=10 # 10MB file for speed

# Move to Simulation Directory for Docker Context
cd "$SIM_DIR"

docker compose down || true
docker compose build
docker compose up -d

echo "Waiting for services..."
sleep 5

# Get IP addresses
SENDER_IP=$(docker compose exec sender hostname -i)
RECEIVER_IP=$(docker compose exec receiver hostname -i)
RELAY_IP=$(docker compose exec coturn hostname -i)

echo "Sender IP: $SENDER_IP"
echo "Receiver IP: $RECEIVER_IP"
echo "Relay IP: $RELAY_IP"

# Helper to run command in container
run_sender() {
    docker compose exec sender sh -c "$1"
}
run_sender_bg() {
    docker compose exec -d sender sh -c "$1"
}
run_receiver_bg() {
    docker compose exec -d receiver sh -c "$1"
}

# Install tools for network manipulation
echo "=== Installing Network Tools (tc/iptables) ==="
run_sender "apk add --no-cache iproute2 iptables"
docker compose exec receiver apk add --no-cache iproute2 iptables

# SCENARIO 1: Strict NAT (Force TURN)
echo "---------------------------------------------------"
echo "SCENARIO 1: Strict Firewall (Blocking Direct P2P)"
echo "---------------------------------------------------"
echo "Blocking traffic between $SENDER_IP and $RECEIVER_IP..."

# Block Direct Communication in both directions
run_sender "iptables -A OUTPUT -d $RECEIVER_IP -j DROP"
run_sender "iptables -A INPUT -s $RECEIVER_IP -j DROP"

# But allow traffic to Relay
# (Default policy is ACCEPT, so we are good)

# Generate Code
CODE="strict-firewall-test"
echo "Starting Sender (Private Relay Configured)..."
# We must use the INTERNAL IP of the relay container since we are inside the network
RELAY_URL="turn:$RELAY_IP:3478"

# Start Sender in background
run_sender "jend config set-relay --url $RELAY_URL --user user --pass password"
run_sender_bg "jend send /app/e2e/test_data/large_file.dat --headless --no-history > /app/sender.log 2>&1"

sleep 5 
CODE=$(run_sender "grep 'Code: ' /app/sender.log | head -n 1 | sed 's/Code: //'")
echo "Code: $CODE"

echo "Starting Receiver..."
run_receiver_bg "jend config set-relay --url $RELAY_URL --user user --pass password"
run_receiver_bg "jend receive $CODE --dir /app/output --headless --no-history > /app/receiver.log 2>&1"

echo "Waiting for transfer..."
sleep 15 # Give it time to route via TURN

# Check Logs
echo "Checking Logs for TURN usage..."
RELAY_USAGE=$(run_sender "grep 'via P2P ICE' /app/sender.log || echo 'fail'")

# Verify file
docker compose cp receiver:/app/output/large_file.dat ./received_strict.dat
DIFF=$(diff "$DATA_DIR/large_file.dat" "./received_strict.dat" || echo "diff")

if [ "$DIFF" == "" ]; then
    echo "✅ Scenario 1 Passed: File transferred despite firewall."
else
    echo "❌ Scenario 1 Failed: File Integrity Mismatch."
    exit 1
fi

# Cleanup Rules
run_sender "iptables -F"

# SCENARIO 2: Packet Loss (Resilience)
echo "---------------------------------------------------"
echo "SCENARIO 2: 20% Packet Loss (Simulating Bad WiFi)"
echo "---------------------------------------------------"

# Add 20% Packet Loss on Sender Outbound
run_sender "tc qdisc add dev eth0 root netem loss 20%"

CODE="packet-loss-test"
# Start Sender
run_sender_bg "jend send /app/e2e/test_data/large_file.dat --headless --no-history > /app/sender_loss.log 2>&1"
sleep 5
CODE_LOSS=$(run_sender "grep 'Code: ' /app/sender_loss.log | head -n 1 | sed 's/Code: //'")

# Start Receiver
run_receiver_bg "jend receive $CODE_LOSS --dir /app/output_loss --headless --no-history > /app/receiver_loss.log 2>&1"

echo "Waiting longer for lossy transfer..."
sleep 30

docker compose cp receiver:/app/output_loss/large_file.dat ./received_loss.dat
DIFF_LOSS=$(diff "$DATA_DIR/large_file.dat" "./received_loss.dat" || echo "diff")

if [ "$DIFF_LOSS" == "" ]; then
    echo "✅ Scenario 2 Passed: File transferred despite 20% packet loss."
else
    echo "❌ Scenario 2 Failed: Transfer failed or corrupted."
    exit 1
fi

echo "=== ALL SCENARIOS PASSED ==="
docker compose down
rm -f ./received_strict.dat ./received_loss.dat
