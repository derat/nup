#!/bin/sh -e
gcloud app --project=$(./project_id.sh) deploy
