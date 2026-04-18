# --- Go build stage ---
FROM golang:1.26-alpine AS go-builder

RUN apk add --no-cache git

WORKDIR /src

# Cache dependencies
COPY server/go.mod server/go.sum ./server/
RUN cd server && go mod download

# Copy server source
COPY server/ ./server/

# Build binaries
ARG VERSION=dev
ARG COMMIT=unknown
RUN cd server && CGO_ENABLED=0 go build -ldflags "-s -w" -o bin/server ./cmd/server
RUN cd server && CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" -o bin/multica ./cmd/multica
RUN cd server && CGO_ENABLED=0 go build -ldflags "-s -w" -o bin/migrate ./cmd/migrate

# --- Web build stage ---
FROM node:22-alpine AS web-builder

RUN corepack enable

WORKDIR /src

COPY package.json pnpm-lock.yaml pnpm-workspace.yaml .npmrc ./
COPY apps/web/package.json ./apps/web/package.json

RUN pnpm install --frozen-lockfile

COPY apps/web/ ./apps/web/

ENV REMOTE_API_URL=http://127.0.0.1:8081
RUN pnpm --filter @multica/web build

# --- Runtime stage ---
FROM alpine:3.21

RUN apk add --no-cache bash ca-certificates caddy nodejs tzdata

WORKDIR /app

COPY --from=go-builder /src/server/bin/server .
COPY --from=go-builder /src/server/bin/multica .
COPY --from=go-builder /src/server/bin/migrate .
COPY server/migrations/ ./migrations/
COPY --from=web-builder /src/apps/web/.next/standalone/ ./web/
COPY --from=web-builder /src/apps/web/.next/static ./web/apps/web/.next/static
COPY --from=web-builder /src/apps/web/public ./web/apps/web/public
COPY deploy/Caddyfile /etc/caddy/Caddyfile
COPY deploy/run-multica-service.sh /app/run-multica-service.sh

RUN chmod +x /app/run-multica-service.sh

EXPOSE 8080

ENTRYPOINT ["/app/run-multica-service.sh"]
