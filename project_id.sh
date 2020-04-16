#!/bin/sh -e

# Convenience script that prints the GCP project ID after reading it from a
# 'projectId' property in config.json.

if ! command -v jq >/dev/null; then
  echo "jq command not found; install from https://stedolan.github.io/jq/" >&2
  exit 1
fi

project=$(jq -r .projectId <config.json) || exit 1

if [ "$project" = 'null' ]; then
  echo "'projectId' property missing in config.json" >&2
  exit 1
fi

echo -n "$project"
