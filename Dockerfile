# syntax=docker/dockerfile:1
# Two images from one file: `--target api` and `--target ui`.

ARG NODE_IMAGE_TAG=22-alpine
ARG GOLANG_IMAGE_TAG=1.26-alpine
ARG ALPINE_IMAGE_TAG=3.22
ARG NGINX_IMAGE_TAG=1.29-alpine

# ---- ui build
FROM node:${NODE_IMAGE_TAG} AS ui-build
WORKDIR /app
RUN npm i -g pnpm@latest
COPY cwmerch-ui/package.json cwmerch-ui/pnpm-lock.yaml cwmerch-ui/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY cwmerch-ui/ ./
COPY manifest.json ./public/manifest.json
RUN pnpm run build

# ---- api build
FROM golang:${GOLANG_IMAGE_TAG} AS api-build
WORKDIR /app
COPY cwmerch-api/go.mod cwmerch-api/go.sum ./
RUN go mod download
COPY cwmerch-api/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/cwmerch-api .

# ---- api run
FROM alpine:${ALPINE_IMAGE_TAG} AS api
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 cwmerch \
    && mkdir -p /storage /uploads && chown cwmerch /storage /uploads
COPY --from=api-build /out/cwmerch-api /usr/local/bin/cwmerch-api
COPY manifest.json /manifest.json
ENV CWMERCH_MANIFEST_PATH=/manifest.json \
    CWMERCH_AI_DOWNLOAD_DIR=/storage
USER cwmerch
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s CMD wget -qO- http://127.0.0.1:8080/v1/health >/dev/null || exit 1
ENTRYPOINT ["/usr/local/bin/cwmerch-api"]

# ---- ui run
FROM nginx:${NGINX_IMAGE_TAG} AS ui
COPY --from=ui-build /app/dist /usr/share/nginx/html
COPY .docker/nginx/default.conf /etc/nginx/conf.d/default.conf
COPY .docker/nginx/seo-proxy.inc /etc/nginx/conf.d/seo-proxy.inc
COPY .docker/nginx/docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh
ENTRYPOINT ["/docker-entrypoint.sh"]
CMD ["nginx", "-g", "daemon off;"]
