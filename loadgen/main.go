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

type Result struct {
	LatencyMs float64
	TTFTMs    float64
	Error     bool
}

type Output struct {
	QPS           int     `json:"qps"`
	AvgLatencyMs  float64 `json:"avg_latency_ms"`
	P95LatencyMs  float64 `json:"p95_latency_ms"`
	TTFTMs        float64 `json:"ttft_ms"`
	ThroughputRps float64 `json:"throughput_rps"`
	Errors        int     `json:"errors"`
}

func main() {
	qps := flag.Int("qps", 10, "Target QPS")
	concurrency := flag.Int("concurrency", 5, "Number of concurrent workers")
	requests := flag.Int("requests", 100, "Total number of requests to send")
	promptSize := flag.Int("prompt-size", 100, "Approximate size of prompt in characters")
	url := flag.String("url", "http://localhost:8080/v1/chat/completions", "Target URL")
	outDir := flag.String("out", "../results", "Output directory for results.json")
	flag.Parse()

	log.Printf("Starting Load Generator: %d requests @ %d QPS, %d concurrency", *requests, *qps, *concurrency)

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
	results := make(chan Result, *requests)

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
					results <- Result{Error: true}
					continue
				}
				req.Header.Set("Content-Type", "application/json")

				resp, err := client.Do(req)
				if err != nil {
					results <- Result{Error: true}
					continue
				}

				reader := bufio.NewReader(resp.Body)
				var firstTokenTime time.Time
				firstTokenReceived := false

				for {
					line, err := reader.ReadString('\n')
					if err != nil {
						if err != io.EOF {
							results <- Result{Error: true}
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
					results <- Result{Error: true}
				} else {
					results <- Result{LatencyMs: latency, TTFTMs: ttft, Error: false}
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

	sort.Float64s(latencies)

	var avgLatency, p95Latency, avgTTFT float64
	if len(latencies) > 0 {
		sumLatency := 0.0
		for _, l := range latencies {
			sumLatency += l
		}
		avgLatency = sumLatency / float64(len(latencies))
		p95Latency = latencies[int(float64(len(latencies))*0.95)]
	}

	if len(ttfts) > 0 {
		sumTTFT := 0.0
		for _, t := range ttfts {
			sumTTFT += t
		}
		avgTTFT = sumTTFT / float64(len(ttfts))
	}

	durationSec := globalEnd.Sub(globalStart).Seconds()
	throughput := float64(*requests) / durationSec

	out := Output{
		QPS:           *qps,
		AvgLatencyMs:  avgLatency,
		P95LatencyMs:  p95Latency,
		TTFTMs:        avgTTFT,
		ThroughputRps: throughput,
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
