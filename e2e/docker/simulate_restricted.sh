#!/bin/bash
set -e

# Cleanup
rm -f ../../transfer_code.txt
rm -f ../../sender_output.log
rm -rf ../../output
mkdir -p ../../output

echo "Building and Starting Restricted Network Simulation..."
docker-compose -f docker-compose.restricted.yml up --build -d

echo "Waiting for sender to generate code..."
MAX_RETRIES=30
CODE=""

for i in $(seq 1 $MAX_RETRIES); do
    if [ -f "../../sender_output.log" ]; then
        if grep -q "Code: " "../../sender_output.log"; then
           CODE=$(grep "Code: " "../../sender_output.log" | head -n 1 | sed 's/Code: //')
           CODE=$(echo "$CODE" | xargs) # trim
           break
        fi
    fi
    sleep 1
done

if [ -z "$CODE" ]; then
    echo "Timeout waiting for code."
    docker-compose -f docker-compose.restricted.yml logs sender
    docker-compose -f docker-compose.restricted.yml down
    exit 1
fi

echo "Code generated: $CODE"
echo "$CODE" > ../../transfer_code.txt

echo "Waiting for receiver to finish..."
# We can tail logs or wait for a success file, or just wait for container exit.
# Receiver container runs the command then exits.
docker-compose -f docker-compose.restricted.yml wait receiver

echo "Checking results..."
EXIT_CODE=$(docker-compose -f docker-compose.restricted.yml ps -q receiver | xargs docker inspect -f '{{.State.ExitCode}}')

if [ "$EXIT_CODE" == "0" ]; then
    echo "Receiver exited with 0"
    if grep -q "P2P (ICE) Connected" "../../sender_output.log"; then
        echo "SUCCESS: Connection established via Signaling/P2P!"
    else
        echo "WARNING: Check logs to ensure it didn't accidentally use LAN."
    fi
else
    echo "FAILURE: Receiver exited with non-zero code"
fi

docker-compose -f docker-compose.restricted.yml logs
docker-compose -f docker-compose.restricted.yml down
