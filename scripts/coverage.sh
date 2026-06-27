#!/bin/bash
set -e

COVERAGE_FILE="coverage.out"
THRESHOLD=60

echo "Running tests with coverage..."
go test -coverprofile="${COVERAGE_FILE}" ./... > /dev/null 2>&1 || true

if [ ! -f "${COVERAGE_FILE}" ]; then
    echo "Error: Coverage file not generated"
    exit 1
fi

echo ""
echo "=== Coverage Report ==="
go tool cover -func="${COVERAGE_FILE}" | grep "total:" | awk '{print "Total coverage: " $3}'

echo ""
echo "=== Packages Below ${THRESHOLD}% ==="
go tool cover -func="${COVERAGE_FILE}" | grep -v "total:" | awk -v threshold="${THRESHOLD}" '
{
    coverage = $3
    gsub(/%/, "", coverage)
    if (coverage + 0 < threshold + 0) {
        print coverage "%\t" $1
    }
}' | sort -n

rm -f "${COVERAGE_FILE}"

echo ""
echo "=== Done ==="
