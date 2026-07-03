FROM golang:1.24-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/paperless ./cmd/paperless

FROM alpine:3.21

RUN addgroup -S app \
    && adduser -S app -G app \
    && apk add --no-cache ca-certificates poppler-utils tzdata \
    && mkdir -p /app/uploads \
    && chown -R app:app /app
USER app
WORKDIR /app

COPY --from=build /out/paperless /app/paperless

EXPOSE 8080
ENTRYPOINT ["/app/paperless"]
