# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY echo-stream.go .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o server echo-stream.go

# Runtime stage
FROM gcr.io/distroless/static-debian12:latest-nonroot

WORKDIR /app
COPY --from=builder /app/server .

EXPOSE 8080
USER nonroot:nonroot

ENTRYPOINT ["/app/server"]
