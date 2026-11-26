#!/usr/bin/env bash
# Versions and image names shared by the delivery scripts.
export APPS="api ui"
export APP_PREFIX="goocommerce"
export VERSION="$(grep -oE "^[0-9\.]+$" VERSION)"
export VERSION_SHA="${VERSION}-${CI_COMMIT_SHORT_SHA:-local}"

echo "VERSION=${VERSION}" > .env.ci
echo "VERSION_SHA=${VERSION_SHA}" >> .env.ci
