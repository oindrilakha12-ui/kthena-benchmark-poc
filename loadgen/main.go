package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

type ChatCompletionRequest struct {
	Model    string                  `json:"model"`
	Messages []ChatCompletionMessage `json:"messages"`
	Stream   bool                    `json:"stream"`
}

type ChatCompletionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type RequestResult struct {
	LatencyMs float64
	TTFTMs    float64
	Error     bool
}

type PercentileSet struct {
	P50 float64 `json:"p50"`
	P95 float64 `json:"p95"`
	P99 float64 `json:"p99"`
}

type ScenarioConfig struct {
	QPS         float64 `json:"qps"`
	Concurrency int     `json:"concurrency"`
	Duration    string  `json:"duration"`
}

type Result struct {
	Scenario        string         `json:"scenario"`
	Timestamp       string         `json:"timestamp"`
	RoutingStrategy string         `json:"routing_strategy"`
	BackendCount    int            `json:"backend_count"`
	Config          ScenarioConfig `json:"config"`
	Latency         PercentileSet  `json:"latency_ms"`
	TTFT            PercentileSet  `json:"ttft_ms"`
	ThroughputRPS   float64        `json:"throughput_rps"`
	TotalRequests   int            `json:"total_requests"`
	Errors          int            `json:"errors"`
}

func calculatePercentiles(values []float64) PercentileSet {
	if len(values) == 0 {
		return PercentileSet{}
	}
	sort.Float64s(values)
	n := len(values)
	
	p50Idx := int(float64(n) * 0.50)
	p95Idx := int(float64(n) * 0.95)
	p99Idx := int(float64(n) * 0.99)
	
	if p50Idx >= n { p50Idx = n - 1 }
	if p95Idx >= n { p95Idx = n - 1 }
	if p99Idx >= n { p99Idx = n - 1 }
	
	return PercentileSet{
		P50: values[p50Idx],
		P95: values[p95Idx],
		P99: values[p99Idx],
	}
}

func main() {
	qps := flag.Float64("qps", 10.0, "Target QPS")
	concurrency := flag.Int("concurrency", 5, "Number of concurrent workers")
	requests := flag.Int("requests", 100, "Total number of requests to send")
	promptSize := flag.Int("prompt-size", 100, "Approximate size of prompt in characters")
	url := flag.String("url", "http://localhost:8080/v1/chat/completions", "Target URL")
	outDir := flag.String("out", "../results", "Output directory for results.json")
	
	// Structured parameters matching new schema
	scenario := flag.String("scenario", "custom", "Scenario name")
	routingStrategy := flag.String("routing-strategy", "round-robin", "Routing strategy used")
	backendCount := flag.Int("backend-count", 2, "Number of backend instances")
	flag.Parse()

	log.Printf("Starting Load Generator: %d requests @ %.2f QPS, %d concurrency", *requests, *qps, *concurrency)

	prompt := strings.Repeat("A", *promptSize)
	reqBody := ChatCompletionRequest{
		Model:  "mock-model",
		Stream: true,
		Messages: []ChatCompletionMessage{
			{Role: "user", Content: prompt},
		},
	}
	reqBytes, _ := json.Marshal(reqBody)

	jobs := make(chan struct{}, *requests)
	results := make(chan RequestResult, *requests)

	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := &http.Client{Timeout: 30 * time.Second}
			for range jobs {
				start := time.Now()
				
				req, err := http.NewRequest("POST", *url, bytes.NewBuffer(reqBytes))
				if err != nil {
					results <- RequestResult{Error: true}
					continue
				}
				req.Header.Set("Content-Type", "application/json")

				resp, err := client.Do(req)
				if err != nil {
					results <- RequestResult{Error: true}
					continue
				}

				reader := bufio.NewReader(resp.Body)
				var firstTokenTime time.Time
				firstTokenReceived := false

				for {
					line, err := reader.ReadString('\n')
					if err != nil {
						if err != io.EOF {
							results <- RequestResult{Error: true}
						}
						break
					}
					line = strings.TrimSpace(line)
					if line == "" {
						continue
					}
					if strings.HasPrefix(line, "data:") {
						if !firstTokenReceived {
							firstTokenTime = time.Now()
							firstTokenReceived = true
						}
						if strings.Contains(line, "[DONE]") {
							break
						}
					}
				}
				resp.Body.Close()

				end := time.Now()
				latency := end.Sub(start).Seconds() * 1000
				var ttft float64
				if firstTokenReceived {
					ttft = firstTokenTime.Sub(start).Seconds() * 1000
				}

				if resp.StatusCode != http.StatusOK {
					results <- RequestResult{Error: true}
				} else {
					results <- RequestResult{LatencyMs: latency, TTFTMs: ttft, Error: false}
				}
			}
		}()
	}

	globalStart := time.Now()

	// Dispatch jobs
	go func() {
		ticker := time.NewTicker(time.Second / time.Duration(*qps))
		defer ticker.Stop()
		for i := 0; i < *requests; i++ {
			<-ticker.C
			jobs <- struct{}{}
		}
		close(jobs)
	}()

	wg.Wait()
	close(results)
	globalEnd := time.Now()

	// Calculate metrics
	var latencies []float64
	var ttfts []float64
	var errorsCount int

	for res := range results {
		if res.Error {
			errorsCount++
		} else {
			latencies = append(latencies, res.LatencyMs)
			ttfts = append(ttfts, res.TTFTMs)
		}
	}

	durationSec := globalEnd.Sub(globalStart).Seconds()
	throughput := float64(*requests) / durationSec

	latencyPercentiles := calculatePercentiles(latencies)
	ttftPercentiles := calculatePercentiles(ttfts)

	out := Result{
		Scenario:        *scenario,
		Timestamp:       globalStart.Format(time.RFC3339),
		RoutingStrategy: *routingStrategy,
		BackendCount:    *backendCount,
		Config: ScenarioConfig{
			QPS:         *qps,
			Concurrency: *concurrency,
			Duration:    globalEnd.Sub(globalStart).Round(time.Second).String(),
		},
		Latency:       latencyPercentiles,
		TTFT:          ttftPercentiles,
		ThroughputRPS: throughput,
		TotalRequests: *requests,
		Errors:        errorsCount,
	}

	outBytes, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(outBytes))

	os.MkdirAll(*outDir, 0755)
	err := os.WriteFile(fmt.Sprintf("%s/results.json", *outDir), outBytes, 0644)
	if err != nil {
		log.Fatalf("Failed to write results: %v", err)
	}
	log.Println("Results saved successfully.")
}
