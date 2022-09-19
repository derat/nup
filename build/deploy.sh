#!/bin/sh -e

usage=$(cat <<EOF
Usage: $0 [flags]... [gcloud args]...
Deploy the App Engine app to GCP and delete old versions.

Flags:
  -i  Update Datastore indexes instead of deploying app
  -p  GCP project ID to use ("nup projectid" by default)
EOF
)

indexes=
project=
while getopts ip: o; do
  case $o in
    i) indexes=1 ;;
    p) project="$OPTARG" ;;
    \?) echo "$usage" >&2 && exit 2 ;;
  esac
done
shift $(expr $OPTIND - 1)

[ -n "$project" ] || project=$(nup projectid)

if [ -n "$indexes" ]; then
  echo 'Creating new indexes...'
  gcloud beta datastore --project="$project" --quiet indexes create index.yaml
  echo 'Deleting old indexes...'
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
echo 'Deploying app...'
gcloud beta app --project="$project" --quiet deploy "$@"

# Clean up stale versions of the app so they aren't sitting around.
echo 'Deleting old versions...'
versions=$(
  gcloud app --project="$project" versions list |
    sed -nre 's/^default +([^ ]+) +0\.00 .*/\1/p'
)
if [ -n "$versions" ]; then
  gcloud app --project="$project" --quiet versions delete $versions
fi
