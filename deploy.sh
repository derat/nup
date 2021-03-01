#!/bin/sh -e

project=$(./project_id.sh)
gcloud app --project="$project" deploy "$@"

# Clean up stale versions of the app so they aren't sitting around.
versions=$(
  gcloud app --project="$project" versions list |
    sed -nre 's/^default +([^ ]+) +0\.00 .*/\1/p'
)
if [ -n "$versions" ]; then
  gcloud app --project="$project" versions delete $versions
fi
