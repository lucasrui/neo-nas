FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o /usb-backup ./cmd/main.go

FROM alpine:latest
COPY --from=builder /usb-backup /usr/local/bin/
ENTRYPOINT ["usb-backup"] 