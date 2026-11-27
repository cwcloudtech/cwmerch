#!/usr/bin/env bash

export APPS="ui api"
export APP_PREFIX="cwmerch"
export VERSION="$(grep -oE "^[0-9\.]+$" VERSION)"
export VERSION_SHA="${VERSION}-${CI_COMMIT_SHORT_SHA}"

echo "VERSION=${VERSION}" > .env.ci
echo "VERSION_SHA=${VERSION_SHA}" >> .env.ci
