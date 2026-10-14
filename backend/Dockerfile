# Image backend berisi seluruh binary (api, worker, migrate, cutover, storefront); proses dipilih lewat
# command di Compose. Versi Go dikunci sama dengan go.mod (Technical Guideline 3.1, 14.3).
FROM golang:1.27.2-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOOS=linux GOFLAGS=-trimpath

COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG VERSION=dev
ARG COMMIT=unknown
RUN mkdir -p /out && go build \
      -ldflags "-s -w -X balans/internal/platform/config.Version=${VERSION} -X balans/internal/platform/config.Commit=${COMMIT}" \
      -o /out/ ./cmd/...

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/ /app/
USER nonroot:nonroot
ENV PATH=/app
EXPOSE 8080
ENTRYPOINT []
CMD ["api"]
