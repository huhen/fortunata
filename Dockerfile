# Сборка статического бинарника.
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/generator ./cmd/server \
 && mkdir -p /out/data && chown 65532:65532 /out/data

# Минимальный рантайм: только бинарник и пустой /data с владельцем 65532
# (иначе named volume при первой инициализации получит root:root и БД не создастся).
FROM scratch
COPY --from=build /out/generator /generator
COPY --from=build --chown=65532:65532 /out/data /data
ENV DB_PATH=/data/generator.db ADDR=:8080
VOLUME /data
EXPOSE 8080
USER 65532:65532
ENTRYPOINT ["/generator"]
