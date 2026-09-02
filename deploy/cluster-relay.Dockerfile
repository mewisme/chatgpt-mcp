FROM node:24-alpine AS assets
WORKDIR /src
COPY . .
RUN npm install -g pnpm@11 && pnpm --dir web install --frozen-lockfile && pnpm --dir web build && node scripts/prepare-web-embed.mjs

FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=assets /src/internal/web/dist ./internal/web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/cgm ./

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
COPY --from=build /out/cgm /usr/local/bin/cgm
ENV HOME=/tmp
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/cgm"]
CMD ["cluster", "relay", "--help"]