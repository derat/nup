# Stage 0: Install Cloud SDK and Chrome from Debian packages.
# Also install the chromedriver executable.

FROM golang:bullseye

# "gcloud builds submit" tars up and sends files from the local directory, but
# I'm not sure where (if anywhere) they're actually available within the image
# as it's being built. Copy this script over so we can run it later.
COPY install_chromedriver.sh /tmp/install_chromedriver.sh

# The Chrome stuff is based on https://askubuntu.com/a/510186 and
# https://github.com/puppeteer/puppeteer/blob/main/docs/troubleshooting.md#running-puppeteer-in-docker
RUN apt-get update
RUN apt-get upgrade
RUN apt-get install -y --no-install-recommends gnupg wget
RUN wget -q -O- https://dl-ssl.google.com/linux/linux_signing_key.pub | apt-key add -
RUN echo 'deb [arch=amd64] http://dl.google.com/linux/chrome/deb/ stable main' \
    >>/etc/apt/sources.list.d/chrome.list
RUN wget -q -O- https://packages.cloud.google.com/apt/doc/apt-key.gpg | apt-key add -
RUN echo 'deb [arch=amd64] https://packages.cloud.google.com/apt cloud-sdk main' \
    >>/etc/apt/sources.list.d/cloud-sdk.list
RUN apt-get update
RUN apt-get install -y \
    fonts-noto-core \
    fonts-noto-mono \
    fonts-noto-ui-core \
    google-chrome-stable \
    google-cloud-sdk \
    google-cloud-sdk-app-engine-go \
    google-cloud-sdk-app-engine-python \
    google-cloud-sdk-datastore-emulator \
    npm \
    python-is-python2 \
    unzip \
    xvfb
RUN npm install -g typescript@4.7.2
RUN sh -e /tmp/install_chromedriver.sh
