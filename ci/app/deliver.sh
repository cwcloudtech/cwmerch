#!/usr/bin/env bash
# Delivery only: builds the api and ui images and pushes them to the
# registry (version, version-sha and latest tags). There is deliberately no
# deployment step: whoever runs goocommerce pulls the images themselves.
set -euo pipefail

source ./ci/app/compute-env.sh

sha="$(git rev-parse --short HEAD)"
details="$(git log --pretty=format:"%an, %ar : %s" -1)"
# Escape backslashes then double quotes (jq isn't available in CI).
details="${details//\\/\\\\}"
details="${details//\"/\\\"}"
echo '{"version":"'"${VERSION}"'", "sha":"'"${sha}"'", "details":"'"${details}"'"}' > manifest.json

docker login "${CI_REGISTRY}" --username "${CI_REGISTRY_USER}" --password "${CI_REGISTRY_PASSWORD}"

for app in ${APPS}; do
  image="${CI_REGISTRY}/${APP_PREFIX}-${app}"
  docker buildx bake -f docker-compose-build.yml --push "${app}"
  if [[ "${VERSION}" != "${VERSION_SHA}" ]]; then
    docker tag "${image}:${VERSION}" "${image}:${VERSION_SHA}"
    docker push "${image}:${VERSION_SHA}"
  fi
  docker tag "${image}:${VERSION}" "${image}:latest"
  docker push "${image}:latest"
done
