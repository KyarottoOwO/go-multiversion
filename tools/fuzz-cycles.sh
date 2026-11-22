#!/usr/bin/env bash
# Runs consecutive fuzzing cycles for one target and fails on the first crash.
# Fuzzing a package with the default parallelism exhausts memory on small
# machines, so the worker count is fixed at 2 unless FUZZ_PARALLEL is set.
#
# usage: tools/fuzz-cycles.sh <package> <target> <minutes> [cycles]
#
# Example: tools/fuzz-cycles.sh ./internal/compat FuzzAdapterDecode 30 3
set -u

pkg=${1:?package required}
target=${2:?fuzz target required}
minutes=${3:?minutes per cycle required}
cycles=${4:-3}
parallel=${FUZZ_PARALLEL:-2}

# This checkout may inherit a global build tag that the default build does not
# support; fuzzing always uses the documented default configuration.
export GOFLAGS="-tags="

for cycle in $(seq 1 "$cycles"); do
	echo "=== ${target} cycle ${cycle}/${cycles} start $(date -Is) ==="
	if ! go test "$pkg" -run '^$' -fuzz="^${target}\$" -fuzztime="${minutes}m" -parallel="$parallel" -count=1; then
		echo "=== ${target} cycle ${cycle} FAILED $(date -Is) ==="
		exit 1
	fi
	echo "=== ${target} cycle ${cycle} clean $(date -Is) ==="
done
echo "=== ${target} all ${cycles} cycles clean $(date -Is) ==="
