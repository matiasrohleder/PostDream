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
ARG VERSION=0.2.0
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/postdream ./cmd/postdream

# ---------- 3a. Standalone: PostDream + its own PostgreSQL in one container ----------
# docker build --target standalone .   ->   docker run -p 8025:8025 -p 1025:1025 <image>
# No login and no external database: made for local use, tests and CI. Data lives in /data (mount a volume to keep it).
FROM postgres:18-alpine AS standalone
RUN apk add --no-cache ca-certificates tzdata \
 && mkdir -p /data /run/postgresql && chown -R postgres:postgres /data /run/postgresql
COPY --from=api /out/postdream /usr/local/bin/postdream
COPY deploy/standalone/entrypoint.sh /usr/local/bin/postdream-standalone
ENV AUTH_ENABLED=false \
    POP3_ADDR=:1110
USER postgres
WORKDIR /data
EXPOSE 1025 1110 8025
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s CMD wget -qO- http://127.0.0.1:8025/api/health || exit 1
STOPSIGNAL SIGTERM
ENTRYPOINT ["postdream-standalone"]

# ---------- 3b. Runtime (default target): PostDream only, PostgreSQL is external ----------
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata && adduser -D -H -u 10001 postdream
COPY --from=api /out/postdream /usr/local/bin/postdream
USER postdream
EXPOSE 1025 8025
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s CMD wget -qO- http://127.0.0.1:8025/api/health || exit 1
ENTRYPOINT ["postdream"]
