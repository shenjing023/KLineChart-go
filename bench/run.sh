#!/usr/bin/env bash
# Regenerate the kline benchmark matrix.
# Usage: ./bench/run.sh [output]
# The default output is bench/baseline.txt.
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
out=${1:-bench/baseline.txt}
mkdir -p "$(dirname "$out")"
# -run '^$' skips tests. -count would repeat them, and
# TestCompareScalesAllocationBudget calls testing.Benchmark, which
# consumes the one-time goos/cpu header before these results print.
go test ./kline -run '^$' -bench . -benchmem -count=6 | tee "$out"
