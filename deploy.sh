#!/bin/sh -e

project=$(./project_id.sh)

# Surprisingly, --quiet only disables yes/no prompts (which we want)
# rather than suppressing output (which we don't want).
gcloud app --project="$project" --quiet deploy "$@"

# Clean up stale versions of the app so they aren't sitting around.
versions=$(
  gcloud app --project="$project" versions list |
    sed -nre 's/^default +([^ ]+) +0\.00 .*/\1/p'
)
if [ -n "$versions" ]; then
  gcloud app --project="$project" --quiet versions delete $versions
fi
