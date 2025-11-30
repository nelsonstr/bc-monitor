# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o main cmd/main.go

# Run stage
FROM alpine:3.19

WORKDIR /app

COPY --from=builder /app/main .
COPY .env.sample .env

# Expose port if needed (though this is a monitor, maybe metrics?)
# EXPOSE 8080

CMD ["./main"]
