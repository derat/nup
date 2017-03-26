#!/bin/sh -e

APP_HOST=localhost
APP_PORT=8080
APP_URL="http://${APP_HOST}:${APP_PORT}"
APP_AUTH=testuser:testpass

FILE_PORT=8123

UPDATE_MUSIC="${GOPATH}/bin/update_music"

curl -u "${APP_AUTH}" -X POST "${APP_URL}/clear"
curl -u "${APP_AUTH}" --data @app_config.json "${APP_URL}/config"

"${UPDATE_MUSIC}" --require-covers \
  --config update_config.json \
  --import-json-file songs.txt
rm -f last_update_time.json

python -m SimpleHTTPServer ${FILE_PORT} 2>/dev/null
