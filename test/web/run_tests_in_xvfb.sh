#!/bin/sh -e
exec xvfb-run -s ':99 -ac -shmem -screen 0 1600x1200x16' ./tests.py "$@"
