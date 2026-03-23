# Conduit

Conduit is a lightweight, high-performance HTTP load balancer written in Go. It utilizes a least-connections strategy to distribute incoming traffic across multiple backend servers efficiently.

## Features

- Least-Connections Load Balancing: Uses a min-heap to always route requests to the backend with the fewest active connections.
- Active Health Checks: Automatically monitors backend availability every 20 seconds and removes unhealthy nodes from the rotation.
- Automatic Retries: Supports up to 3 retries for failed requests before marking a backend as down.
- Dynamic Recovery: Backends are automatically reintegrated into the pool once they pass health checks.
- Docker Ready: Includes a multi-stage Dockerfile and Docker Compose configuration for easy deployment.

## Getting Started

### Prerequisites

- Go 1.25.7 or higher
- Docker and Docker Compose (optional)

### Installation

1. Clone the repository:
   ```bash
   git clone https://github.com/Masralai/conduit.git
   cd conduit
   ```

2. Build the application:
   ```bash
   go build -o conduit server.go
   ```

### Running Locally

Start the load balancer by specifying the backend servers as a comma-separated list:

```bash
./conduit --backends "http://localhost:8081,http://localhost:8082" --port 3030
```

### Running with Docker

Use Docker Compose to spin up the load balancer along with a set of sample web servers:

```bash
docker compose up --build
```

The load balancer will be available at `http://localhost:3030`.

## Configuration

The application accepts the following command-line flags:

| Flag | Description | Default |
|------|-------------|---------|
| `--backends` | Comma-separated list of backend URLs (required) | "" |
| `--port` | The port the load balancer will listen on | 3030 |

## Architecture

Conduit is built using Go's standard library. Key components include:

- ServerPool: Manages the state of all backends and coordinates selection.
- ServerHeap: A min-priority queue (using `container/heap`) that tracks active connections for O(log n) selection and updates.
- ReverseProxy: Leverages `httputil.ReverseProxy` for robust request forwarding and error handling.
- HealthChecker: A background goroutine that performs periodic TCP dial tests to ensure backend liveness.
