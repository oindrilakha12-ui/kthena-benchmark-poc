package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

// Config holds the configuration for the mock backend
type Config struct {
	Port            string
	BackendName     string
	ResponseDelayMs int
	TTFTDelayMs     int
}

func loadConfig() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	backendName := os.Getenv("BACKEND_NAME")
	if backendName == "" {
		backendName = "mock-backend"
	}

	responseDelayMs, _ := strconv.Atoi(os.Getenv("RESPONSE_DELAY_MS"))
	if responseDelayMs == 0 {
		responseDelayMs = 50 // Default 50ms between tokens
	}

	ttftDelayMs, _ := strconv.Atoi(os.Getenv("TTFT_DELAY_MS"))
	if ttftDelayMs == 0 {
		ttftDelayMs = 200 // Default 200ms Time To First Token
	}

	return Config{
		Port:            port,
		BackendName:     backendName,
		ResponseDelayMs: responseDelayMs,
		TTFTDelayMs:     ttftDelayMs,
	}
}

type ChatCompletionRequest struct {
	Model    string                  `json:"model"`
	Messages []ChatCompletionMessage `json:"messages"`
	Stream   bool                    `json:"stream"`
}

type ChatCompletionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatCompletionChunk struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		Index        int         `json:"index"`
		FinishReason *string     `json:"finish_reason"`
	} `json:"choices"`
}

func main() {
	cfg := loadConfig()

	http.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req ChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		// Only support streaming for this PoC as it's the primary way LLMs are consumed
		if !req.Stream {
			http.Error(w, "Only streaming is supported in this mock", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
			return
		}

		// Simulate Time To First Token (TTFT)
		time.Sleep(time.Duration(cfg.TTFTDelayMs) * time.Millisecond)

		tokens := []string{"Hello", " from", " ", cfg.BackendName, ".", " This", " is", " a", " mock", " response", " stream."}

		for i, token := range tokens {
			chunk := ChatCompletionChunk{
				ID:      "chatcmpl-mock",
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Model:   req.Model,
			}
			
			choice := struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				Index        int     `json:"index"`
				FinishReason *string `json:"finish_reason"`
			}{}
			
			choice.Delta.Content = token
			choice.Index = 0
			
			if i == len(tokens)-1 {
				reason := "stop"
				choice.FinishReason = &reason
			}
			
			chunk.Choices = append(chunk.Choices, choice)

			data, _ := json.Marshal(chunk)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()

			if i < len(tokens)-1 {
				time.Sleep(time.Duration(cfg.ResponseDelayMs) * time.Millisecond)
			}
		}
		
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	})

	log.Printf("Starting %s on port %s (TTFT: %dms, Token Delay: %dms)\n", 
		cfg.BackendName, cfg.Port, cfg.TTFTDelayMs, cfg.ResponseDelayMs)
	if err := http.ListenAndServe(":"+cfg.Port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
