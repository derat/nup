#!/bin/sh -e

exec dev_appserver.py --application=nup --datastore_consistency_policy=consistent .
