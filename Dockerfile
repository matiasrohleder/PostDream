# ---------- 1. React UI ----------
# Build stages run on the build machine's architecture and cross-compile (fast multi-arch builds).
FROM --platform=$BUILDPLATFORM node:22-alpine AS ui
WORKDIR /ui
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY frontend/ ./
RUN npm run build

# ---------- 2. Go binary (UI embedded) ----------
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS api
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN rm -rf web/dist
COPY --from=ui /ui/dist ./web/dist
ARG VERSION=0.1.0
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/postdream ./cmd/postdream

# ---------- 3. Runtime ----------
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata && adduser -D -H -u 10001 postdream
COPY --from=api /out/postdream /usr/local/bin/postdream
USER postdream
EXPOSE 1025 8025
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s CMD wget -qO- http://127.0.0.1:8025/api/health || exit 1
ENTRYPOINT ["postdream"]
