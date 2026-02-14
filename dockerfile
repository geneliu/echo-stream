# Build stage
FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod ./
RUN go mod download && go mod verify

COPY *.go ./

ARG TARGETOS
ARG TARGETARCH

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -a -trimpath -ldflags "-s -w" -o server echo-stream.go


# Runtime stage
FROM --platform=$TARGETPLATFORM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app
COPY --from=builder /app/server .

EXPOSE 8080
USER nonroot:nonroot

ENTRYPOINT ["/app/server"]
