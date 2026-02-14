# Echo Stream

A minimalist streaming echo service written in Go for testing network throughput and latency.

## Features

- **Upload Endpoint**: `/upload` - Streams request body to discard (for upload testing)
- **Download Endpoint**: `/download` - Generates streaming response with configurable size
- **Health Check**: `/health` - Simple health endpoint
- **Security**: Built-in rate limiting, request size limits, and graceful shutdown
- **Container Ready**: Multi-stage Docker build with distroless base image

## Endpoints

### POST /upload
Stream data to the server (data is discarded).

```bash
curl -X POST -d @largefile.bin http://localhost:8080/upload
```

### GET /download?size=N
Download N bytes of generated data.

```bash
# Download 1MB
curl -o output.bin "http://localhost:8080/download?size=1048576"

# Download default 2MB
curl -o output.bin "http://localhost:8080/download"
```

### GET /health
Health check endpoint.

```bash
curl http://localhost:8080/health
```

## Security Features

- Request size limits (10MB for uploads, 100MB for downloads)
- Timeout enforcement (30 seconds)
- Graceful shutdown handling
- Context-aware request cancellation
- Buffer overflow protection

## Building

### Local Build
```bash
go build -o echo-stream echo-stream.go
./echo-stream
```

### Docker Build
```bash
docker build -t echo-stream .
docker run -p 8080:8080 echo-stream
```

## Configuration

Environment variables:
- `PORT` - Server port (default: 8080)

## Deployment

See `deploy.yaml` for Kubernetes deployment example.

## License

MIT