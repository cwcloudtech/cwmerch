#!/bin/sh
set -e

if [ "$1" = "nginx" ]; then
    front_app_path="/usr/share/nginx/html"
    conf_path="/etc/nginx/conf.d"

    # 1. Runtime config for the browser: the public API URL.
    api_url="${GOOCOMMERCE_API_URL:-http://localhost:8080}"
    escaped=$(printf '%s' "${api_url}" | sed 's/\\/\\\\/g; s/"/\\"/g')
    printf 'window.__GOOCOMMERCE__ = { apiUrl: "%s" };\n' "${escaped}" > "${front_app_path}/config.js"

    # 2. Internal API URL nginx proxies SEO pages, sitemap.xml and robots.txt to.
    export GOOCOMMERCE_API_INTERNAL_URL="${GOOCOMMERCE_API_INTERNAL_URL:-http://api:8080}"
    GOOCOMMERCE_API_INTERNAL_URL="${GOOCOMMERCE_API_INTERNAL_URL%/}"
    for file in "${conf_path}/default.conf" "${conf_path}/seo-proxy.inc"; do
        if [ -f "${file}" ]; then
            envsubst '${GOOCOMMERCE_API_INTERNAL_URL}' < "${file}" > "${file}.tmp"
            mv "${file}.tmp" "${file}"
        fi
    done
fi

exec "$@"
