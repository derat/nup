#!/bin/sh -e

# This implements https://chromedriver.chromium.org/downloads/version-selection
# to install the latest version of ChromeDriver that's compatible with the
# current google-chrome binary.

# Directory into which the chromedriver binary will be installed.
destdir=/usr/local/bin

# google-chrome --version prints a line like "Google Chrome 96.0.4664.93 " (yes,
# with a trailing space): https://www.chromium.org/developers/version-numbers/
# To get the latest corresponding Chrome driver release, we need
# "MAJOR.MINOR.BUILD", e.g. "96.0.4664".
cver=$(google-chrome --version)
build=$(echo "$cver" | sed -nre 's/^Google Chrome ([0-9]+\.[0-9]+\.[0-9]+)\.[0-9]+\s*$/\1/p')
if [ -z "$build" ]; then
  echo "Failed parsing Chrome version '${cver}'"
  exit 1
fi

# Get the file containing the latest compatible Chromedriver release number.
rel=$(wget --quiet -O- "https://chromedriver.storage.googleapis.com/LATEST_RELEASE_${build}")
echo "Need ChromeDriver ${rel} for Chrome ${cver}"

# Now download Chromedriver itself.
url="https://chromedriver.storage.googleapis.com/${rel}/chromedriver_linux64.zip"
tmpfile=$(mktemp -t "chromedriver.XXXXXXXXXX.zip")
echo "Downloading ${url} to ${tmpfile}"
wget --quiet -O "$tmpfile" "$url"

# Extract the executable.
echo "Extracting chromedriver executable to ${destdir}"
unzip -q "$tmpfile" chromedriver -d "$destdir"
chmod 0755 "${destdir}/chromedriver"
rm -f "$tmpfile"
