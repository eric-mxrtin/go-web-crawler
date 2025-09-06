package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strings"
	"time"
)

const (
	openAIEmbeddingsURL = "https://api.openai.com/v1/embeddings"
	openAIModel         = "text-embedding-3-large" // 8191 token limit, 3072 dimensions
	userAgent           = "ModernFamilyCrawler/1.0 (+https://example.com/bot)"
)

type openAIEmbeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type openAIEmbeddingResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
}

// generate embedding vector for text using openai api
func GetEmbedding(ctx context.Context, apiKey, text string) ([]float64, error) {
	if apiKey == "" {
		return nil, errors.New("OPENAI_API_KEY is not set")
	}

	text = strings.TrimSpace(text)
	if text == "" {
		return nil, errors.New("empty text for embedding")
	}

	// openai will handle token limits automatically
	reqBody := openAIEmbeddingRequest{
		Model: openAIModel,
		Input: []string{text},
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", openAIEmbeddingsURL, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)

	// make http request with retry logic
	httpClient := &http.Client{Timeout: 20 * time.Second}
	resp, err := doRequestWithRetry(req, httpClient)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// check for api errors
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("openai api returned status code %d: %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("embedding api error: %s - %s", resp.Status, string(body))
	}

	var parsed openAIEmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("failed to parse embedding response: %w", err)
	}

	if len(parsed.Data) == 0 || len(parsed.Data[0].Embedding) == 0 {
		return nil, errors.New("empty embedding vector returned")
	}

	return parsed.Data[0].Embedding, nil
}

// normalize vector to unit length for better similarity calculations
func L2Normalize(v []float64) []float64 {
	var sum float64
	for _, x := range v {
		sum += x * x
	}
	if sum == 0 {
		return v
	}
	norm := math.Sqrt(sum)
	out := make([]float64, len(v))
	for i := range v {
		out[i] = v[i] / norm
	}
	return out
}

// http request with exponential backoff retry logic
func doRequestWithRetry(req *http.Request, client *http.Client) (*http.Response, error) {
	const maxRetries = 3
	const initialBackoff = 500 * time.Millisecond

	var err error
	var resp *http.Response
	backoff := initialBackoff

	for attempt := 1; attempt <= maxRetries; attempt++ {
		resp, err = client.Do(req)
		if err == nil && resp.StatusCode >= 200 && resp.StatusCode <= 299 {
			return resp, nil
		}
		// Close body if non-nil to avoid resource leak before retry.
		if resp != nil && resp.Body != nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
		if attempt < maxRetries {
			time.Sleep(backoff)
			backoff *= 2
		}
	}
	if err == nil && resp != nil {
		return resp, fmt.Errorf("non-2xx status after retries: %s", resp.Status)
	}
	return nil, err
}
