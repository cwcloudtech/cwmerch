# Shared by the SEO locations of default.conf. The original query string is
# kept after path= (e.g. /v1/seo/html?path=/shop&page=2) and the full
# original URI is also sent as X-Original-URI.
proxy_pass ${GOOCOMMERCE_API_INTERNAL_URL};
proxy_set_header Host $host;
proxy_set_header X-Original-URI $request_uri;
proxy_set_header X-Forwarded-Host $host;
proxy_set_header X-Forwarded-Proto $scheme;
proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
proxy_set_header Accept-Encoding "";
proxy_connect_timeout 2s;
proxy_read_timeout 5s;
proxy_intercept_errors on;
error_page 404 500 502 503 504 = /index.html;
add_header Cache-Control "no-cache";
