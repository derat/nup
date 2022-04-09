#!/bin/sh -e

if [ "$1" = '-h' ] || [ "$1" = '--help' ]; then
  cat <<EOF >&2
Usage: deploy.sh [flags]... [gcloud args]...
Deploy the App Engine app to GCP and delete old versions.

Flags:
  -i, --indexes    Deploy datastore indexes rather than app
EOF
  exit 2
fi

project=$(nup projectid)

if [ "$1" = '-i' ] || [ "$1" = '--indexes' ]; then
  gcloud beta datastore --project="$project" --quiet indexes create index.yaml
  gcloud beta datastore --project="$project" --quiet indexes cleanup index.yaml
  exit 0
fi

# As of November 2021, 'beta' is required here to use App Engine bundled
# services (e.g. memcache) from the go115 runtime. Without it, the 'deploy'
# command prints 'WARNING: There is a dependency on App Engine APIs, but they
# are not enabled in your app.yaml. Set the app_engine_apis property.' even when
# the property is set.
#
# Surprisingly, --quiet only disables yes/no prompts (which we want) rather than
# suppressing output (which we don't want).
gcloud beta app --project="$project" --quiet deploy "$@"

# Clean up stale versions of the app so they aren't sitting around.
versions=$(
  gcloud app --project="$project" versions list |
    sed -nre 's/^default +([^ ]+) +0\.00 .*/\1/p'
)
if [ -n "$versions" ]; then
  gcloud app --project="$project" --quiet versions delete $versions
fi
