#!/bin/bash
set -e

# Cleanup previous runs
rm -f bin/jend
rm -f sender.log
rm -rf output_test
mkdir -p output_test

echo "Building jend..."
go build -o bin/jend ./cmd/jend

# Create dummy file
echo "Hello AWS JEND World $(date)" > xyz_aws_payload.txt

echo "Starting Sender in S3/AWS mode..."
# Run sender in background with --s3
./bin/jend send xyz_aws_payload.txt --headless --no-history --s3 > sender.log 2>&1 &
SENDER_PID=$!

echo "Sender running with PID $SENDER_PID. Waiting for code..."

CODE=""
for i in {1..30}; do
    if grep -q "Code: " sender.log; then
        CODE=$(grep "Code: " sender.log | head -n 1 | sed 's/Code: //')
        # Trim whitespace
        CODE=$(echo "$CODE" | xargs)
        break
    fi
    sleep 1
done

if [ -z "$CODE" ]; then
    echo "TIMEOUT: Could not get code from sender."
    cat sender.log
    kill $SENDER_PID || true
    exit 1
fi

echo "Code received: '$CODE'"
# Wait for the file to actually be uploaded and registered
sleep 5

echo "Starting Receiver..."
./bin/jend receive "$CODE" --headless --no-history --dir output_test

if [ -f "output_test/xyz_aws_payload.txt" ]; then
    echo "SUCCESS: File received via AWS mode!"
    diff xyz_aws_payload.txt output_test/xyz_aws_payload.txt
else
    echo "FAILURE: File not found in output directory."
    ls -l output_test
    exit 1
fi

# Cleanup
kill $SENDER_PID || true
rm xyz_aws_payload.txt
rm sender.log
rm -rf output_test
