FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o /neo-nas ./cmd/main.go

FROM alpine:latest
RUN apk add --no-cache tzdata
ENV TZ=Asia/Shanghai
COPY --from=builder /neo-nas /usr/local/bin/
ENTRYPOINT ["neo-nas"] 