#!/bin/bash
set -e

REPO_ROOT=$(git rev-parse --show-toplevel)
cd "$REPO_ROOT"

RUN_DOCKER=false


# Parse Arguments
for arg in "$@"; do
    case $arg in
        --docker)
            RUN_DOCKER=true
            shift
            ;;
        *)
            ;;
    esac
done

echo "========================================"
echo "   JEND Test Runner"
echo "========================================"

# 1. Cleanup any potential zombie processes from previous runs
echo "[*] Cleaning up any stale jend processes..."
pkill -f "jend" || true
pkill -f "jend_test" || true
sleep 2 # Ensure ports are released

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

# 4. Optional: Docker Tests
if [ "$RUN_DOCKER" = true ]; then
    echo ""
    echo "[*] Running Docker E2E Tests..."
    if [ -f "./e2e/docker/test_runner.sh" ]; then
        chmod +x ./e2e/docker/test_runner.sh
        ./e2e/docker/test_runner.sh
    else
        echo "Error: Docker test runner not found at ./e2e/docker/test_runner.sh"
        exit 1
    fi
fi

echo ""
echo "========================================"
echo "   All Tests Passed Successfully! 🚀"
echo "========================================"

# Final Cleanup (Double check)
rm -rf test_data output received_large
rm -f *.bin *.txt *.mp4 jend_test_* jend
echo "[*] Workspace verified clean."
