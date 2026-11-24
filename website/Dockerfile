# Documentation site: build the static Docusaurus site, serve it with Caddy.
# Build context is website/ (Railway root directory), so only the docs are sent.
FROM node:22-alpine AS build
WORKDIR /site
COPY package.json package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY . .
ARG DOCS_URL
ENV DOCS_URL=${DOCS_URL}
RUN npm run build

FROM caddy:2-alpine
COPY Caddyfile /etc/caddy/Caddyfile
COPY --from=build /site/build /srv
ENV PORT=8080
EXPOSE 8080
