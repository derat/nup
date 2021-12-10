#!/bin/bash

set -e

# This script runs in /workspace, which also contains the checked-out
# repository. PATH appears to default to the following:
# /go/bin:/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

go version
google-chrome --version

# Some tests need to run update_music and dump_music.
go install ./cmd/...

# Let test.CloudBuild() detect that it's running on Cloud Build.
export CLOUD_BUILD=1

# Create a directory for tests to write to.
tmpdir=$(mktemp -t -d nup.XXXXXXXXXX)
export OUTPUT_DIR="${tmpdir}/${BUILD_ID}"

# Cloud Build YAML files support copying artifacts to Cloud Storage, but they
# infuriatingly don't actually get saved if the build fails:
# https://stackoverflow.com/questions/61487234/storing-artifacts-from-a-failed-build
# https://issuetracker.google.com/issues/128353446
# https://github.com/GoogleCloudPlatform/cloud-builders/issues/253
#
# They also don't seem to pass -r to gsutil, apparently requiring you to specify
# individual files rather than directories.
function copy_artifacts {
  local tarball="${BUILD_ID}.tgz"
  local dest="gs://${PROJECT_ID}-artifacts/nup-test/${tarball}"
  cd "$tmpdir"
  tar czf "$tarball" "$BUILD_ID"
  echo "Copying output to ${dest}"
  gsutil cp "$tarball" "$dest"
}
trap copy_artifacts EXIT

# Run at most one command (e.g. test executable) at a time since slow VMs may
# have trouble running multiple dev_appserver.py instances simultaneously.
go test -v -p 1 ./...
