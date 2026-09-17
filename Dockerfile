# Сборка статического бинарника.
FROM golang:1.26-alpine AS build
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X fortunata/internal/version.Version=${VERSION}" -o /out/fortunata ./cmd/server \
 && mkdir -p /out/data && chown 65532:65532 /out/data

# Минимальный рантайм: только бинарник и пустой /data с владельцем 65532
# (иначе named volume при первой инициализации получит root:root и БД не создастся).
# CA-бандл нужен для исходящего HTTPS (синхронизация с timelottery.ru);
# без него Go не проверит сертификаты: x509: unknown authority.
FROM scratch
COPY --from=build /out/fortunata /fortunata
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build --chown=65532:65532 /out/data /data
ENV DB_PATH=/data/fortunata.db ADDR=:8080
VOLUME /data
EXPOSE 8080
USER 65532:65532
ENTRYPOINT ["/fortunata"]
