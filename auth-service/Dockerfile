FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY EXBanka-3-Backend/auth-service/ ./EXBanka-3-Backend/auth-service/
WORKDIR /app/EXBanka-3-Backend/auth-service
RUN go build -o /auth-service ./cmd/server

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /auth-service ./auth-service
EXPOSE 8081 9091
CMD ["./auth-service"]
