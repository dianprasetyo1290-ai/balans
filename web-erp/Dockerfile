# web-erp dibangun menjadi file static dan dilayani Caddy di dalam image (Technical Guideline 14.3).
# Konteks build adalah root repository karena tipe API digenerate dari backend/api.
FROM node:24.19.0-alpine AS build
WORKDIR /repo/web-erp
RUN corepack enable

COPY web-erp/package.json web-erp/pnpm-lock.yaml web-erp/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile

COPY backend/api /repo/backend/api
COPY web-erp/ ./
# Tidak ada nilai per lingkungan di build: image yang sama dipakai staging dan production.
ARG VERSION=dev
ARG SENTRY_DSN=""
ENV VITE_APP_VERSION=${VERSION} VITE_SENTRY_DSN=${SENTRY_DSN}
RUN pnpm build

FROM caddy:2.11.4-alpine
COPY web-erp/Caddyfile /etc/caddy/Caddyfile
COPY --from=build /repo/web-erp/dist /srv
EXPOSE 8080
