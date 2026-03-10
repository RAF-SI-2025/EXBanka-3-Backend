FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod ./
COPY . .

RUN go build -o employee-service .

FROM alpine:latest

WORKDIR /root/

COPY --from=builder /app/employee-service .

EXPOSE 8080

CMD ["./employee-service"]