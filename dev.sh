#!/bin/bash -e

# dev_appserver.py host and port.
host=localhost
port=8080

# Whether various flags are set.
example=
tests=

# Destinations for noisy dev_appserver.py output.
srvout=/dev/stdout
srverr=/dev/stderr

while [ "$#" -gt 0 ]; do
  case "$1" in
    -e|--example)
      example=1
      ;;
    -t|--test)
      tests=1
      srvout=/dev/null
      srverr=/dev/null
      ;;
    *)
      cat <<EOF >&2
Usage: dev.sh [flags]
Start development App Engine instance and optionally load example data.

Flags:
  -h, --help     Display this message.
  -e, --example  Load example data into server.
  -t, --test     Run all tests and exit.
EOF
      exit 2
      ;;
  esac
  shift 1
done

# On SIGINT or SIGTERM, send SIGTERM to our process group (including ourselves).
# I think that killing via session ID may be more reliable, but sessions are
# created by terminals and unavailable when running as a script.
trap "trap - SIGTERM && pkill -g $$" SIGINT SIGTERM

# When we exit normally, also send SIGTERM to the group, but ignore it so we can
# preserve the original exit code.
function cleanup() {
  local code=$?
  trap '' SIGTERM
  pkill -g $$
  trap - EXIT
  exit $code
}
trap cleanup EXIT

echo "Starting dev_appserver.py..." >&2
dev_appserver.py --application=nup \
  --datastore_consistency_policy=consistent . \
  >$srvout 2>$srverr &

# Wait for dev_appserver.py to start accepting connections:
# https://stackoverflow.com/a/27601038/6882947
while ! nc -z "$host" "$port"; do sleep 0.1; done
echo "dev_appserver.py is accepting connections at ${host}:${port}" >&2

if [ -n "$tests" ]; then
  echo "Running all tests..." >&2
  go test -p 1 -count 1 ./...
  exit $?
elif [ -n "$example" ]; then
  echo "Loading example data..." >&2
  example/import_to_devserver.sh &
fi

sleep infinity
