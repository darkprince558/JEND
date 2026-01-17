
#!/bin/bash
set -e

# Setup directories
cd "$(dirname "$0")"
ROOT_DIR="../.."
TEST_DATA_DIR="$ROOT_DIR/e2e/test_data"
OUTPUT_DIR="$ROOT_DIR/output"

# Cleanup
rm -f "$ROOT_DIR/transfer_code.txt"
rm -f "$ROOT_DIR/sender_output.log"
rm -rf "$OUTPUT_DIR"
mkdir -p "$TEST_DATA_DIR"
mkdir -p "$OUTPUT_DIR"

# Create Payload
echo "Hello from Docker!" > "$TEST_DATA_DIR/payload.txt"

echo "=== Building Containers ==="
docker-compose build

echo "=== Starting Sender ==="
docker-compose up -d sender

echo "Waiting for sender to generate code..."
TIMEOUT=30
ELAPSED=0
CODE=""

while [ $ELAPSED -lt $TIMEOUT ]; do
    if [ -f "$ROOT_DIR/sender_output.log" ]; then
        # Grep for Code: X-Y-Z
        CODE=$(grep "Code: " "$ROOT_DIR/sender_output.log" | head -n 1 | sed 's/Code: //')
        if [ ! -z "$CODE" ]; then
            break
        fi
    fi
    sleep 1
    ELAPSED=$((ELAPSED+1))
done

if [ -z "$CODE" ]; then
    echo "Error: Timed out waiting for code."
    docker-compose logs sender
    docker-compose down
    exit 1
fi

echo "Got Code: $CODE"
echo "$CODE" > "$ROOT_DIR/transfer_code.txt"

echo "=== Starting Receiver ==="
docker-compose up -d receiver

echo "Waiting for receiver to finish..."
# Wait for receiver container to exit
docker-compose wait receiver

echo "=== Verifying Transfer ==="
if [ -f "$OUTPUT_DIR/payload.txt" ]; then
    CONTENT=$(cat "$OUTPUT_DIR/payload.txt")
    if [ "$CONTENT" == "Hello from Docker!" ]; then
        echo "SUCCESS: File transferred correctly!"
    else
        echo "FAILURE: Content mismatch. Got: '$CONTENT'"
        EXIT_CODE=1
    fi
else
    echo "FAILURE: Output file not found."
    EXIT_CODE=1
fi

echo "=== Logs ==="
docker-compose logs

echo "=== Cleanup ==="
docker-compose down
rm -f "$ROOT_DIR/transfer_code.txt"
rm -f "$ROOT_DIR/sender_output.log"
rm -f "$TEST_DATA_DIR/payload.txt"

exit ${EXIT_CODE:-0}
