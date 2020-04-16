#!/bin/sh -e
project=$(./project_id.sh)
versions=$(
  gcloud app --project="$project" versions list |
    sed -nre 's/^default +([^ ]+) +0\.00 .*/\1/p'
)
if [ -z "$versions" ]; then
  echo "No old versions" >&2
  exit 0
fi
gcloud app --project="$project" versions delete $versions
