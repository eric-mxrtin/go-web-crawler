// main.go
package main

import (
	"bytes"
	"context"
	"crawler/scraper"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"

	"github.com/PuerkitoBio/goquery"
	"github.com/temoto/robotstxt"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// ========================= Constants & Config =========================

const (
	startURL            = "https://modernfamily.fandom.com/wiki/Pilot"
	userAgent           = "ModernFamilyCrawler/1.0 (+https://example.com/bot)" // customize if you want
	defaultWorkers      = 6
	defaultHTTPTimeout  = 20 * time.Second
	maxRetries          = 3
	initialBackoff      = 500 * time.Millisecond
	openAIEmbeddingsURL = "https://api.openai.com/v1/embeddings"
	openAIModel         = "text-embedding-3-small" // small, cheap; alter if you prefer
)

func loadEnv() {
	// Load .env file if it exists
	if _, err := os.Stat(".env"); err == nil {
		if err := godotenv.Load(); err != nil {
			log.Fatal("Error loading .env file")
		}
	}

	// Verify required variables
	required := []string{"MONGO_URI", "OPENAI_API_KEY"}
	for _, key := range required {
		if os.Getenv(key) == "" {
			log.Printf("WARNING: Environment variable %s not set", key)
		}
	}
}

// ========================= Data Models =========================

// EpisodeDoc is the MongoDB schema for an episode document.
type EpisodeDoc struct {
	ID             primitive.ObjectID `bson:"_id,omitempty"`
	URL            string             `bson:"url"`
	PageTitle      string             `bson:"page_title"`
	EpisodeContent string             `bson:"episode_content"`
	Embedding      []float64          `bson:"embedding"` // store as float64 array for BSON
}

// ========================= Globals =========================

var httpClient = &http.Client{Timeout: defaultHTTPTimeout}

// ========================= Utilities =========================

func saveEpisodesToMarkdown(episodes []EpisodeDoc, filename string) error {
	var builder strings.Builder
	builder.WriteString("# Modern Family Episode Summaries\n\n")

	for _, ep := range episodes {
		builder.WriteString(fmt.Sprintf("## [%s](%s)\n\n", ep.PageTitle, ep.URL))
		builder.WriteString(ep.EpisodeContent)
		builder.WriteString("\n\n---\n\n")
	}

	return os.WriteFile(filename, []byte(builder.String()), 0644)
}

// doRequestWithRetry performs an HTTP request with up to maxRetries and exponential backoff.
func doRequestWithRetry(req *http.Request) (*http.Response, error) {
	var err error
	var resp *http.Response
	backoff := initialBackoff

	for attempt := 1; attempt <= maxRetries; attempt++ {
		resp, err = httpClient.Do(req)
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

// fetchRobots fetches and parses robots.txt for a host.
func fetchRobots(robotsURL string) (*robotstxt.RobotsData, error) {
	req, err := http.NewRequest("GET", robotsURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := doRequestWithRetry(req)
	if err != nil {
		return nil, fmt.Errorf("robots.txt request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return robotstxt.FromStatusAndBytes(resp.StatusCode, body)
}

// isAllowed checks if a URL is allowed by robots for our user-agent.
func isAllowed(robots *robotstxt.RobotsData, rawURL string) bool {
	if robots == nil {
		return false
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	gr := robots.FindGroup(userAgent)
	if gr == nil {
		// If no specific group, use default (*)
		gr = robots.FindGroup("*")
	}
	return gr.Test(u.Path)
}

// absoluteURL resolves relative hrefs against a base.
func absoluteURL(base, href string) (string, error) {
	bu, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	hu, err := url.Parse(href)
	if err != nil {
		return "", err
	}
	return bu.ResolveReference(hu).String(), nil
}

// textFromSelection concatenates text from selected elements, trimming whitespace.
func textFromSelection(sel *goquery.Selection) string {
	var parts []string
	sel.Each(func(i int, s *goquery.Selection) {
		t := strings.TrimSpace(s.Text())
		if t != "" {
			parts = append(parts, t)
		}
	})
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

// ========================= Embeddings =========================

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

// getEmbedding calls a REST embeddings API (OpenAI-compatible) and returns a vector.
func getEmbedding(ctx context.Context, apiKey, text string) ([]float64, error) {
	if apiKey == "" {
		return nil, errors.New("OPENAI_API_KEY is not set")
	}

	text = strings.TrimSpace(text)
	if text == "" {
		return nil, errors.New("empty text for embedding")
	}

	// OpenAI embedding models have a max token limit.
	// The "text-embedding-3-small" model has a max input length of 8191 tokens.
	// Truncate the text to prevent oversized requests.
	// A simple character limit is used here as a proxy for token count.

	// Create the request payload
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

	resp, err := doRequestWithRetry(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read the body to get the error message
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		// Read the body to get the error message
		log.Printf("OpenAI API returned status code %d: %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("embedding API error: %s - %s", resp.Status, string(body))
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

// ========================= MongoDB =========================

type mongoEnv struct {
	Client       *mongo.Client
	DB           *mongo.Database
	Collection   *mongo.Collection
	VectorIndex  string
	OpenAIAPIKey string
}

func initMongo(ctx context.Context) (*mongoEnv, error) {
	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		return nil, errors.New("MONGO_URI is not set")
	}

	// Create a new client and connect to the server
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		return nil, err
	}

	// Ping the primary
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		return nil, err
	}

	dbName := os.Getenv("MONGO_DB")
	if dbName == "" {
		dbName = "modern_family"
	}

	return &mongoEnv{
		Client:       client,
		DB:           client.Database(dbName),
		Collection:   client.Database(dbName).Collection("episodes"),
		VectorIndex:  "embedding_index", // or os.Getenv("MONGO_VECTOR_INDEX")
		OpenAIAPIKey: os.Getenv("OPENAI_API_KEY"),
	}, nil
}

func saveEpisode(ctx context.Context, env *mongoEnv, doc *EpisodeDoc) error {
	if doc.URL == "" {
		return errors.New("missing URL")
	}
	_, err := env.Collection.UpdateOne(
		ctx,
		bson.M{"url": doc.URL},
		bson.M{
			"$set": bson.M{
				"url":             doc.URL,
				"page_title":      doc.PageTitle,
				"episode_content": doc.EpisodeContent,
				"embedding":       doc.Embedding,
			},
		},
		options.Update().SetUpsert(true), // Fixed options package
	)
	return err
}

// performSearch runs a vector search over MongoDB Atlas using $vectorSearch.
func performSearch(ctx context.Context, env *mongoEnv, query string) error {
	if strings.TrimSpace(query) == "" {
		return errors.New("empty query")
	}
	vec, err := getEmbedding(ctx, env.OpenAIAPIKey, query)
	if err != nil {
		return fmt.Errorf("failed to embed query: %w", err)
	}

	// $vectorSearch stage (Atlas): adjust numCandidates/limit as needed.
	pipeline := mongo.Pipeline{
		{{
			Key: "$vectorSearch",
			Value: bson.D{
				{Key: "index", Value: env.VectorIndex},
				{Key: "path", Value: "embedding"},
				{Key: "queryVector", Value: vec},
				{Key: "numCandidates", Value: 100},
				{Key: "limit", Value: 5},
			},
		}},
		{{Key: "$project", Value: bson.D{
			{Key: "page_title", Value: 1},
			{Key: "url", Value: 1},
			{Key: "_id", Value: 0},
			{Key: "score", Value: bson.D{{Key: "$meta", Value: "vectorSearchScore"}}},
		}}},
	}

	cur, err := env.Collection.Aggregate(ctx, pipeline)
	if err != nil {
		return fmt.Errorf("aggregate vector search error: %w", err)
	}
	defer cur.Close(ctx)

	type res struct {
		PageTitle string  `bson:"page_title"`
		URL       string  `bson:"url"`
		Score     float64 `bson:"score"`
	}
	fmt.Println("Top results:")
	i := 0
	for cur.Next(ctx) {
		i++
		var r res
		if err := cur.Decode(&r); err != nil {
			log.Printf("decode error: %v", err)
			continue
		}
		fmt.Printf("%d) %s\n   %s\n   score: %.4f\n\n", i, r.PageTitle, r.URL, r.Score)
	}
	if i == 0 {
		fmt.Println("No results.")
	}
	return nil
}

// worker consumes episode URLs, fetches & parses the page, embeds content, and stores it.
// worker consumes episode URLs, fetches & parses the page, embeds content, and stores it.
func worker(ctx context.Context, id int, env *mongoEnv, robots *robotstxt.RobotsData, jobs <-chan string, wg *sync.WaitGroup) {
	defer wg.Done()
	for u := range jobs {
		if !isAllowed(robots, u) {
			log.Printf("[worker %d] blocked by robots.txt: %s", id, u)
			continue
		}

		title, summary, err := scraper.ExtractEpisodeData(ctx, u)
		if err != nil {
			log.Printf("[worker %d] extract error for %s: %v", id, u, err)
			continue
		}

		// Use a more robust text cleaning method.
		// First, check for valid UTF-8 and clean.
		summary = strings.ToValidUTF8(summary, "")

		// Remove control characters (except newline, tab, etc., which are often fine).
		// This regex pattern matches and removes non-printable ASCII characters.
		re := regexp.MustCompile("[\x00-\x1F\x7F]")
		summary = re.ReplaceAllString(summary, "")

		summary = strings.TrimSpace(summary)

		if summary == "" {
			log.Printf("[worker %d] skipping empty summary after cleaning for %s", id, u)
			continue
		}

		// Add this check to prevent oversized requests
		const maxCharLimit = 8000 // A bit less than 8191 to be safe
		if len(summary) > maxCharLimit {
			summary = summary[:maxCharLimit]
			log.Printf("[worker %d] truncated summary for %s to %d characters", id, u, maxCharLimit)
		}

		// Add log for debugging the payload content
		log.Printf("[worker %d] embedding request: len=%d, preview=%q", id, len(summary), summary[:min(50, len(summary))])

		vec, err := getEmbedding(ctx, env.OpenAIAPIKey, summary)
		if err != nil {
			log.Printf("[worker %d] embedding error for %s: %v", id, u, err)
			continue
		}
		vec = l2Normalize(vec)

		doc := &EpisodeDoc{
			URL:            u,
			PageTitle:      title,
			EpisodeContent: summary,
			Embedding:      vec,
		}
		if err := saveEpisode(ctx, env, doc); err != nil {
			log.Printf("[worker %d] mongo save error for %s: %v", id, u, err)
			continue
		}
		log.Printf("[worker %d] saved: %s", id, title)
		time.Sleep(200 * time.Millisecond)
	}
}

// l2Normalize normalizes a vector to unit length (helps some vector indices).
func l2Normalize(v []float64) []float64 {
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

// uniqueStrings de-duplicates a slice while preserving order.
func uniqueStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}

// ========================= Main (Crawl / Search CLI) =========================
func main() {
	loadEnv()
	workers := flag.Int("workers", defaultWorkers, "number of concurrent workers")
	search := flag.String("search", "", "semantic search query (when set, crawlers won't run)")
	flag.Parse()

	_ = workers
	_ = search

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	env, err := initMongo(ctx)
	if err != nil {
		log.Fatalf("mongo init error: %v", err)
	}
	defer func() {
		_ = env.Client.Disconnect(context.Background())
	}()

	if strings.TrimSpace(*search) != "" {
		if err := performSearch(ctx, env, *search); err != nil {
			log.Fatalf("search error: %v", err)
		}
		return
	}

	if env.OpenAIAPIKey == "" {
		log.Fatal("OPENAI_API_KEY is not set")
	}

	robotsTargets := []string{
		"https://fandom.com/robots.txt",
		"https://www.fandom.com/robots.txt",
		"https://modernfamily.fandom.com/robots.txt",
	}
	var robots *robotstxt.RobotsData
	for _, rURL := range robotsTargets {
		rb, err := fetchRobots(rURL)
		if err == nil && rb != nil {
			if strings.Contains(rURL, "modernfamily.fandom.com") {
				robots = rb
				break
			}
			robots = rb
		}
	}
	if robots == nil {
		log.Fatal("failed to retrieve robots.txt; aborting to be safe")
	}

	if !isAllowed(robots, startURL) {
		log.Fatalf("robots.txt disallows fetching: %s", startURL)
	}

	episodeLinks, err := scraper.ExtractEpisodeLinks(ctx, startURL)
	if err != nil {
		log.Fatalf("failed to parse episode links: %v", err)
	}
	if len(episodeLinks) == 0 {
		log.Fatal("no episode links found; check selector or page structure")
	}

	// Allocate a buffered channel to prevent the main goroutine from blocking
	jobs := make(chan string, len(episodeLinks))
	var wg sync.WaitGroup

	workerCount := *workers
	if workerCount < 1 {
		workerCount = 1
	}
	log.Printf("Starting %d workers…", workerCount)

	// This block correctly starts the workers
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go worker(ctx, i+1, env, robots, jobs, &wg)
	}

	// This block correctly sends the jobs to the channel
	for _, link := range episodeLinks {
		jobs <- link
	}
	close(jobs) // Crucial to close the channel after all jobs are sent

	// This correctly waits for the workers to finish after the channel is closed
	wg.Wait()
	log.Println("Crawl completed.")
}

/*
========================= Vector Index (MongoDB Atlas) =========================

Create a vector search index on the "episodes" collection.
In Atlas UI -> Collections -> Indexes -> "Create Search Index" (JSON Editor) and use:

{
  "mappings": {
    "dynamic": false,
    "fields": {
      "embedding": {
        "type": "knnVector",
        "dimensions": 1536,
        "similarity": "cosine"
      }
    }
  }
}

Name it the same as your env var MONGO_VECTOR_INDEX (default "embedding_index").
If you use a different embedding model, adjust "dimensions" accordingly.

===============================================================================
*/
