#!/bin/bash
set -e

# Cleanup
rm -rf testdir test_out output_e2e_* *.tar.gz *.zip

# Setup
echo "Creating test data..."
mkdir -p testdir/subdir
echo "content1" > testdir/f1
echo "content2" > testdir/subdir/f2
echo "filecontent" > testfile.txt

# Test 1: Directory (Auto Tar)
echo "-----------------------------------"
echo "TRUE TEST 1: Directory Send (Auto Tar)"
echo "-----------------------------------"
go run cmd/jend/main.go --headless send testdir > sender.log 2>&1 &
SENDER_PID=$!
sleep 2
CODE=$(grep "Code:" sender.log | awk '{print $2}')
echo "Code: $CODE"
go run cmd/jend/main.go --headless --unzip --dir output_e2e_1 receive $CODE
wait $SENDER_PID

if [ -f "output_e2e_1/testdir/f1" ]; then
    echo "TEST 1 PASSED"
else
    echo "TEST 1 FAILED"
    exit 1
fi

# Test 2: File with --zip
echo "-----------------------------------"
echo "TRUE TEST 2: File Send --zip"
echo "-----------------------------------"
go run cmd/jend/main.go --headless --zip send testfile.txt > sender.log 2>&1 &
SENDER_PID=$!
sleep 2
CODE=$(grep "Code:" sender.log | awk '{print $2}')
echo "Code: $CODE"
go run cmd/jend/main.go --headless --unzip --dir output_e2e_2 receive $CODE
wait $SENDER_PID

if [ -f "output_e2e_2/testfile.txt" ]; then
    echo "TEST 2 PASSED"
else
    echo "TEST 2 FAILED"
    exit 1
fi

echo "ALL E2E TESTS PASSED"
