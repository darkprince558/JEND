#!/bin/bash
set -e

echo "========================================"
echo "   JEND Test Runner"
echo "========================================"

# 1. Cleanup any potential zombie processes from previous runs
echo "[*] Cleaning up any stale jend processes..."
pkill -f "jend" || true
pkill -f "jend_test" || true

# 2. Run Unit Tests (Root and internal packages, excluding e2e)
echo ""
echo "[*] Running Unit Tests..."
go test ./internal/... ./pkg/... ./cmd/...
echo "Unit Tests Passed."

# 3. Run End-to-End Tests
echo ""
echo "[*] Running E2E Tests (this may take a minute)..."
# -v for verbose output to see progress
# -count=1 to disable caching and force a fresh run
go test -v -count=1 ./e2e

echo ""
echo "========================================"
echo "   All Tests Passed Successfully! 🚀"
echo "========================================"

# Final Cleanup (Double check)
rm -rf test_data output received_large
rm -f *.bin *.txt *.mp4 jend_test_* jend
echo "[*] Workspace verified clean."
