FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY EXBanka-3-Backend/ ./EXBanka-3-Backend/
WORKDIR /app/EXBanka-3-Backend
RUN go build -o /server ./cmd/server

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /server ./server
EXPOSE 8080 9090
CMD ["./server"]
