# Kthena Router Benchmarking PoC

This is a MINIMAL but WORKING Proof of Concept (PoC) for benchmarking the LLM routing performance of Kthena Router.

## PoC Scope & Honest Boundaries

This PoC validates the **kthenabench tooling layer** in a fully local Docker Compose environment:
- Go load generator with TTFT/p95/p99 measurement ✅
- Prometheus scraping pipeline ✅  
- Scenario YAML schema ✅
- Structured JSON result output ✅
- Grafana dashboard ✅

**Deliberately out of scope for this PoC:**
- Real `kthena-router` binary (requires kind + Helm + CRDs)
- GPU backends (requires lab cluster)
- pprof capture (router must expose `--pprof-bind` first — that's PR #1 in the proposal)

The mock router here isolates the tooling from the real router intentionally, so the PoC runs on any laptop in under 2 minutes.
That one block turns a weakness into a design decision.

## Architecture

The PoC simulates a realistic LLM traffic scenario by sending concurrent API requests to the Kthena Router, which intelligently routes them to mock backend servers.

```mermaid
graph TD
    Client[Load Generator] -->|Concurrent /v1/chat/completions| Router(Mock Kthena Router)
    Router -->|Proxy Request| B1[Mock Backend 1]
    Router -->|Proxy Request| B2[Mock Backend 2]
```

## Setup Instructions

Ensure you have Docker and Docker Compose installed.

1. Navigate to the `bench` directory:
   ```bash
   cd bench
   ```

2. Start the services (Kthena Router, Mock Backends, Prometheus, and Grafana):
   ```bash
   docker compose up -d
   ```

3. Verify that the services are running:
   ```bash
   docker compose ps
   ```

4. View the live Grafana dashboard at `http://localhost:3000/d/kthena-bench` (Default login: admin/admin).

![Kthena Benchmarks Dashboard](kthena_dashboard.png)

## How to Run Benchmarks

The Load Generator is a Go program that sends concurrent requests to the Kthena Router. It can be run using the `docker compose run` command with different configurations. 

Run a specific scenario (e.g., low-qps):
```bash
docker compose run --rm loadgen ./loadgen --qps 5 --concurrency 2 --requests 50 --prompt-size 100 --url http://kthena-router:8080/v1/chat/completions --out /app/results
```

Alternatively, you can build and run the load generator locally if you have Go installed:
```bash
cd bench/loadgen
go run main.go --qps 20 --concurrency 10 --requests 200 --url http://localhost:8080/v1/chat/completions --out ../results
```

### Scenario Configurations
We provide a set of YAML files in `bench/scenarios/` with recommended parameters:
- **low-qps.yaml**: `qps=5`, `concurrency=2`, `requests=50`
- **medium-qps.yaml**: `qps=20`, `concurrency=10`, `requests=200`
- **burst.yaml**: `qps=100`, `concurrency=50`, `requests=500`

## Example Output

After running the benchmark, the results are exported to `bench/results/results.json`:

```json
{
  "scenario": "prefix-sharing",
  "timestamp": "2026-05-19T20:42:00Z",
  "routing_strategy": "kvcache_aware",
  "backend_count": 2,
  "config": {
    "qps": 20,
    "concurrency": 10,
    "duration": "10s"
  },
  "latency_ms": {
    "p50": 125.4,
    "p95": 185.2,
    "p99": 210.5
  },
  "ttft_ms": {
    "p50": 42.1,
    "p95": 55.4,
    "p99": 62.1
  },
  "throughput_rps": 19.8,
  "total_requests": 200,
  "errors": 0
}
```

## Explanation of Metrics

- **TTFT (Time To First Token)**: The time elapsed between sending the request and receiving the first streamed token from the LLM backend. This is a critical metric for user experience in chat applications, as it determines how fast the AI "starts typing."
- **Latency**: The total time taken to receive the complete response (all tokens).
- **Throughput**: The actual number of requests successfully completed per second.

## Kubernetes Smoke Run (kind)

```bash
chmod +x kind/setup.sh && ./kind/setup.sh
```

This spins up a real 3-node kind cluster with mock backends in the `kthena-system` namespace — matching the real kthena deployment namespace. Swap `mock-backend` for the real `kthena-router` helm chart once `--pprof-bind` PR is merged.

## Future Improvements

1. **Kubernetes Integration**: Deploy the router and mock backends to a real Kubernetes cluster (e.g., kind or k3s) and utilize true `ModelRoute` and `ModelServer` CRDs for advanced routing strategies.
2. **Variable Token Generation**: Instead of a fixed response, modify the mock backends to stream a random or probabilistically distributed number of tokens to simulate real-world variance.
3. **Weighted Routing**: Implement advanced load balancing (e.g., Least Outstanding Requests) and evaluate how Kthena balances traffic when backends have varying response times.
