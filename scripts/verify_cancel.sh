#!/bin/bash
# Verify Sender Cancellation

# Setup
mkdir -p tmp_verify
SRC="tmp_verify/large.bin"
OUT="tmp_verify/out"
rm -rf $OUT
mkdir -p $OUT

# Create 500MB file
dd if=/dev/zero of=$SRC bs=1M count=500

# Build
go build -o tmp_verify/jend ./cmd/jend

# Start Sender
echo "Starting Sender..."
./tmp_verify/jend send $SRC --headless --timeout 30s > tmp_verify/sender.log 2>&1 &
SENDER_PID=$!
echo "Sender PID: $SENDER_PID"

# Wait for code
sleep 2
CODE=$(grep "Code: " tmp_verify/sender.log | awk '{print $2}')
if [ -z "$CODE" ]; then
    echo "Failed to get code"
    cat tmp_verify/sender.log
    kill $SENDER_PID
    exit 1
fi
echo "Code: $CODE"

# Start Receiver
echo "Starting Receiver..."
./tmp_verify/jend receive $CODE --dir $OUT --headless > tmp_verify/receiver.log 2>&1 &
RECEIVER_PID=$!
echo "Receiver PID: $RECEIVER_PID"

# Wait for transfer to start
sleep 2

# Check if receiver is running
if ! ps -p $RECEIVER_PID > /dev/null; then
   echo "Receiver died early!"
   cat tmp_verify/receiver.log
   kill $SENDER_PID
   exit 1
fi

# Send SIGINT to Sender
echo "Interrupting Sender..."
kill -SIGINT $SENDER_PID

# Wait for Receiver to exit
# It should receive error and exit.
wait $RECEIVER_PID
EXIT_CODE=$?
echo "Receiver Exit Code: $EXIT_CODE"

# Check logs
echo "--- Receiver Log ---"
cat tmp_verify/receiver.log
echo "--------------------"

if grep -q "transfer cancelled by sender" tmp_verify/receiver.log; then
    echo "SUCCESS: Cancellation message found."
else
    echo "FAILURE: Cancellation message NOT found."
fi

# Cleanup
kill $SENDER_PID 2>/dev/null
rm -rf tmp_verify
