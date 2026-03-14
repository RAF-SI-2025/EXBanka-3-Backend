FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY EXBanka-3-Backend/client-service/ ./EXBanka-3-Backend/client-service/
WORKDIR /app/EXBanka-3-Backend/client-service
RUN go build -o /client-service ./cmd/server

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /client-service ./client-service
EXPOSE 8083 9093
CMD ["./client-service"]
