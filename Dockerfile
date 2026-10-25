# Компилятор и тесты остаются в первом этапе.
FROM golang:1.26.8-alpine3.23@sha256:a8fa79c5bd40d880b52bd3b6d7669ecdcfd00e85facdd427d279efb5ddd79cb1 AS build
WORKDIR /src
COPY go.mod ./
COPY . .
ENV CGO_ENABLED=0
RUN go test ./... && go vet ./...
RUN go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server && \
    go build -trimpath -ldflags="-s -w" -o /out/health-monitor ./cmd/health-monitor

FROM alpine:3.23@sha256:ff7f0c727dd796aa36787bfcd9a64bf35a6ce72a73451b3359fde849b6703d63
LABEL org.opencontainers.image.title="symbol-web-dockerize" \
      org.opencontainers.image.description="Генератор ASCII-надписей и монитор Docker" \
      org.opencontainers.image.source="https://github.com/GhostOfAstana/symbol-web-dockerize" \
      org.opencontainers.image.version="1.0.0" \
      io.tomorrow.project="symbol-web-dockerize"
# Bash нужен для проверки файловой системы из задания; приложение работает без root.
RUN apk add --no-cache bash ca-certificates && \
    addgroup -S -g 10001 app && adduser -S -D -H -u 10001 -G app app
WORKDIR /app
COPY --from=build --chown=app:app /out/server /out/health-monitor ./
COPY --from=build --chown=app:app /src/templates ./templates
COPY --from=build --chown=app:app /src/static ./static
COPY --from=build --chown=app:app /src/standard.txt /src/shadow.txt /src/thinkertoy.txt ./
USER app:app
ENV ADDR=:8080 GOMEMLIMIT=96MiB
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 CMD ["/app/server", "--healthcheck"]
STOPSIGNAL SIGTERM
ENTRYPOINT ["/app/server"]
