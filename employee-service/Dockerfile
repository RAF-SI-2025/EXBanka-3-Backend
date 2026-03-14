FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY EXBanka-3-Backend/employee-service/ ./EXBanka-3-Backend/employee-service/
WORKDIR /app/EXBanka-3-Backend/employee-service
RUN go build -o /employee-service ./cmd/server

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /employee-service ./employee-service
EXPOSE 8082 9092
CMD ["./employee-service"]
