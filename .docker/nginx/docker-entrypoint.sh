#!/bin/sh
set -e

if [ "$1" = "nginx" ]; then
    defined_envs=$(printf '${%s} ' $(env | cut -d= -f1))
    front_app_path="/usr/share/nginx/html"
    for file in $(find "${front_app_path}" -type f -name '*.css' -o -name '*.js' -o -name '*.html')
    do
        envsubst "$defined_envs" < $file > ${file}.tmp
        mv ${file}.tmp $file
    done

    # To prevent cache issues
    sed -i 's!\(/assets/[^"]\+\)\.\(js\|css\)"!\1.\2?t='$(date '+%s')'"!g' "${front_app_path}/index.html"

    # SEO pages are loaded from the API's public URL (the same CWMERCH_API_URL
    # as the browser): split it into origin (scheme://host[:port]) and base path.
    api_url="${CWMERCH_API_URL:-http://localhost:8080}"
    api_url="${api_url%/}"
    rest="${api_url#*://}"
    case "$rest" in
        */*) api_path="/${rest#*/}" ;;
        *) api_path="" ;;
    esac
    api_origin="${api_url%"$api_path"}"
    resolver=$(awk '$1 == "nameserver" { print $2; exit }' /etc/resolv.conf)
    case "$resolver" in
        "") resolver="127.0.0.11" ;;
        *:*) resolver="[$resolver]" ;;
    esac
    esc() { printf '%s' "$1" | sed 's/[\\|&]/\\&/g'; }
    sed -i "s|__API_ORIGIN__|$(esc "$api_origin")|g; s|__API_PATH__|$(esc "$api_path")|g; s|__RESOLVER__|$(esc "$resolver")|g" \
        /etc/nginx/conf.d/default.conf
fi

exec "$@"
