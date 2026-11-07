# syntax=docker/dockerfile:1.7
# ---------- 1. build the web UI ----------
FROM --platform=$BUILDPLATFORM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json* ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# ---------- 2. build the Go binary ----------
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
ARG TARGETOS TARGETARCH VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
COPY third_party/pocket-core ./third_party/pocket-core
RUN cd third_party/pocket-core && sha256sum -c SHA256SUMS
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/pocket-translate ./cmd/pocket-translate

# ---------- 3. runtime ----------
FROM alpine:3.21
# tesseract reads photos and scanned PDFs, poppler's pdftotext reads PDFs
RUN apk add --no-cache ca-certificates tzdata curl poppler-utils tesseract-ocr \
      tesseract-ocr-data-eng tesseract-ocr-data-deu tesseract-ocr-data-fra tesseract-ocr-data-ita tesseract-ocr-data-spa \
      tesseract-ocr-data-por tesseract-ocr-data-nld tesseract-ocr-data-pol tesseract-ocr-data-ces tesseract-ocr-data-slk \
      tesseract-ocr-data-hrv tesseract-ocr-data-srp tesseract-ocr-data-slv tesseract-ocr-data-mkd tesseract-ocr-data-bul \
      tesseract-ocr-data-rus tesseract-ocr-data-ukr tesseract-ocr-data-hun tesseract-ocr-data-ron tesseract-ocr-data-tur \
      tesseract-ocr-data-sqi tesseract-ocr-data-swe tesseract-ocr-data-dan tesseract-ocr-data-nor tesseract-ocr-data-fin \
 && addgroup -g 1000 app && adduser -u 1000 -G app -D -h /data app
COPY --from=build /out/pocket-translate /usr/local/bin/pocket-translate
ENV DATA_DIR=/data PORT=8080 HOST=0.0.0.0
VOLUME ["/data"]
EXPOSE 8080
USER app
WORKDIR /data
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD curl -fsS http://localhost:8080/healthz || exit 1
LABEL org.opencontainers.image.title="Pocket Translate" \
      org.opencontainers.image.description="Translate text, photos, documents and conversations on your own server, with Pocket AI or offline (LibreTranslate and NLLB-200). A phrasebook and a glossary of your own. One Go binary, one SQLite file, one volume." \
      org.opencontainers.image.source="https://github.com/abjelosevic88/pocket-homelab-translate" \
      org.opencontainers.image.licenses="MIT"
ENTRYPOINT ["pocket-translate"]
