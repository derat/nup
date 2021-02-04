#!/bin/sh -e

APP_HOST=localhost
APP_PORT=8080
APP_URL="http://${APP_HOST}:${APP_PORT}"
APP_AUTH=testuser:testpass

FILE_HOST=localhost
FILE_PORT=8123

UPDATE_MUSIC="${GOPATH}/bin/update_music"

curl -u "${APP_AUTH}" -X POST "${APP_URL}/clear"
curl -u "${APP_AUTH}" --data @app_config.json "${APP_URL}/config"

# To test with actual songs, remove --require-covers and --import-json-file and
# update musicDir in update_config.json.
"${UPDATE_MUSIC}" --require-covers \
  --config update_config.json \
  --import-json-file songs.txt
rm -f last_update_time.json

# To test with actual songs, serve their directory rather than '.' and update
# songBaseURL in app_config.json (if the songs aren't in a music/ subdir).
go run http-server.go -addr "${FILE_HOST}:${FILE_PORT}" .
