# Shared by the SEO locations of default.conf: the request goes to the API's
# PUBLIC URL (CWMERCH_API_URL), with its own Host (and TLS SNI). The original
# query string is kept after path= (e.g. ?path=/shop&page=2).
proxy_pass $cwmerch_api;
proxy_ssl_server_name on;
proxy_set_header X-Original-URI $request_uri;
proxy_set_header X-Forwarded-Host $host;
proxy_set_header X-Forwarded-Proto $scheme;
proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
proxy_set_header Accept-Encoding "";
proxy_hide_header Cache-Control;
proxy_connect_timeout 2s;
proxy_read_timeout 5s;
proxy_intercept_errors on;
error_page 404 500 502 503 504 = /index.html;
add_header Cache-Control "no-cache";
