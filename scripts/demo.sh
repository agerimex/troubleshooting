#!/usr/bin/env bash
# make demo: the quality gate passing and then failing, on this machine.
#
# Starts the dev stack (make up), runs the example application's tests
# (examples/quality-gate) twice and checks each run with troubleshooting-check:
#   1. the good version: the gate passes (exit code 0);
#   2. a planted regression (EXAMPLE_REGRESSION=1: prices read one by one, a
#      call to a retired service): every test still passes, the gate fails
#      (exit code 1) and names the requests behind it.
# It writes both reports to .demo/ and prints where to open them.
# Needs Docker with Compose, make and Go. The first start of ClickHouse can
# take several minutes; the images must be built once (make all).
set -euo pipefail
cd "$(dirname "$0")/.."

UI=${DEMO_UI:-http://localhost:8095}
OTLP=${DEMO_OTLP:-http://127.0.0.1:4317}
out=.demo
stamp=$(date +%Y%m%d-%H%M%S)
mkdir -p "$out"

echo "== Starting Troubleshooting (make up)"
make up >/dev/null

# The CLI from this checkout, so the demo needs no go install.
echo "== Building troubleshooting-check"
(cd quality-check && go build -o "../$out/troubleshooting-check" ./cmd/troubleshooting-check)
check="$PWD/$out/troubleshooting-check"

# gate <name> <expected exit code> [env...]: one test run and its check.
gate() {
  local name=$1 want=$2 run="demo-$stamp-$1"; shift 2
  echo
  echo "== $name: the example's tests (run $run)"
  (cd examples/quality-gate && env OTEL_EXPORTER_OTLP_ENDPOINT="$OTLP" OTEL_EXPORTER_OTLP_INSECURE=true \
    OTEL_RESOURCE_ATTRIBUTES="troubleshooting.run=$run" "$@" go test -count=1 ./... | tail -1)
  sleep 3 # the exporter's last batch
  echo "== $name: the quality gate"
  local code=0
  "$check" --url "$UI" --rules examples/quality-gate/quality.yaml --run "$run" \
    --html "$out/$name.html" --markdown "$out/$name.md" || code=$?
  if [ "$code" != "$want" ]; then
    echo "demo: the $name run's gate exited with $code, expected $want" >&2
    exit 1
  fi
}

gate good 0
gate regression 1 EXAMPLE_REGRESSION=1

echo
echo "== Done. The tests passed both times; the gate passed the good version and failed the regression."
echo "   Reports: $out/good.html and $out/regression.html (open them in a browser)."
echo "   The links in the regression's result open the requests behind each failure in $UI."
