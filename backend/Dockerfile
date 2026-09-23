# 构建阶段：只依赖 go.mod / go.sum 先下载依赖，源代码变化时不需要重新下载。
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# 运行阶段：只带二进制，不带源码、不带 .env。
FROM alpine:3.20
RUN adduser -D -u 10001 app \
    && mkdir -p /data/uploads \
    && chown -R app:app /data
WORKDIR /app
COPY --from=build /out/server /app/server
USER app
EXPOSE 8080
ENTRYPOINT ["/app/server"]
