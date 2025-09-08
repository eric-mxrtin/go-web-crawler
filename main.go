// modern family web crawler with semantic search
// crawls episode data, generates embeddings, and stores in mongodb for vector search
package main

import (
	"context"
	"crawler/scraper"
	"crawler/utils"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/joho/godotenv"
	"github.com/temoto/robotstxt"
)

const (
	startURL           = "https://modernfamily.fandom.com/wiki/Pilot"
	userAgent          = "ModernFamilyCrawler/1.0 (+https://example.com/bot)"
	defaultWorkers     = 6
	defaultHTTPTimeout = 20 * time.Second
	maxRetries         = 3
	initialBackoff     = 500 * time.Millisecond
)

func loadEnv() {
	// load .env file if it exists
	if _, err := os.Stat(".env"); err == nil {
		if err := godotenv.Load(); err != nil {
			log.Fatal("error loading .env file")
		}
	}

	// check for required env vars
	required := []string{"MONGO_URI", "OPENAI_API_KEY"}
	for _, key := range required {
		if os.Getenv(key) == "" {
			log.Printf("warning: environment variable %s not set", key)
		}
	}
}

var httpClient = &http.Client{Timeout: defaultHTTPTimeout}

// crawl metrics for performance tracking
type CrawlMetrics struct {
	TotalProcessed      int64
	SuccessfullyCrawled int64
	FailedCrawls        int64
	BlockedByRobots     int64
	EmptyContent        int64
	EmbeddingErrors     int64
	MongoErrors         int64
	StartTime           time.Time
	EndTime             time.Time
}

// http request with exponential backoff retry logic
func doRequestWithRetry(req *http.Request) (*http.Response, error) {
	var err error
	var resp *http.Response
	backoff := initialBackoff

	for attempt := 1; attempt <= maxRetries; attempt++ {
		resp, err = httpClient.Do(req)
		if err == nil && resp.StatusCode >= 200 && resp.StatusCode <= 299 {
			return resp, nil
		}
		// close body to avoid resource leak before retry
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

// fetch and parse robots.txt for a host
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

// check if url is allowed by robots.txt for our user-agent
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
		// if no specific group, use default (*)
		gr = robots.FindGroup("*")
	}
	return gr.Test(u.Path)
}

// resolve relative urls against a base url
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

// extract text from goquery selection, trimming whitespace
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

// worker goroutine that processes episode urls from the job channel
// fetches page content, generates embeddings, and stores in mongodb
func worker(ctx context.Context, id int, env *utils.MongoEnv, robots *robotstxt.RobotsData, jobs <-chan string, wg *sync.WaitGroup, metrics *CrawlMetrics) {
	defer wg.Done()
	for u := range jobs {
		atomic.AddInt64(&metrics.TotalProcessed, 1)

		if !isAllowed(robots, u) {
			log.Printf("[worker %d] blocked by robots.txt: %s", id, u)
			atomic.AddInt64(&metrics.BlockedByRobots, 1)
			continue
		}

		title, summary, err := scraper.ExtractEpisodeData(ctx, u)
		if err != nil {
			log.Printf("[worker %d] extract error for %s: %v", id, u, err)
			atomic.AddInt64(&metrics.FailedCrawls, 1)
			continue
		}

		// clean up the text content
		summary = strings.ToValidUTF8(summary, "")

		// remove control characters but keep newlines and tabs
		re := regexp.MustCompile("[\x00-\x1F\x7F]")
		summary = re.ReplaceAllString(summary, "")
		summary = strings.TrimSpace(summary)

		if summary == "" {
			log.Printf("[worker %d] skipping empty summary after cleaning for %s", id, u)
			atomic.AddInt64(&metrics.EmptyContent, 1)
			continue
		}

		// openai will handle token limits automatically
		log.Printf("[worker %d] embedding request: len=%d, preview=%q", id, len(summary), summary[:min(50, len(summary))])

		vec, err := utils.GetEmbedding(ctx, env.OpenAIAPIKey, summary)
		if err != nil {
			log.Printf("[worker %d] embedding error for %s: %v", id, u, err)
			atomic.AddInt64(&metrics.EmbeddingErrors, 1)
			// if it's a token limit error, we could implement chunking here
			if strings.Contains(err.Error(), "token") || strings.Contains(err.Error(), "length") {
				log.Printf("[worker %d] content too long for %s (length: %d), consider implementing chunking", id, u, len(summary))
			}
			continue
		}
		vec = utils.L2Normalize(vec)

		doc := &utils.EpisodeDoc{
			URL:            u,
			PageTitle:      title,
			EpisodeContent: summary,
			Embedding:      vec,
		}
		if err := utils.SaveEpisode(ctx, env, doc); err != nil {
			log.Printf("[worker %d] mongo save error for %s: %v", id, u, err)
			atomic.AddInt64(&metrics.MongoErrors, 1)
			continue
		}
		atomic.AddInt64(&metrics.SuccessfullyCrawled, 1)
		log.Printf("[worker %d] saved: %s", id, title)
		time.Sleep(200 * time.Millisecond)
	}
}

// remove duplicate strings while preserving order
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

// display crawl performance metrics
func displayMetrics(metrics *CrawlMetrics) {
	metrics.EndTime = time.Now()
	duration := metrics.EndTime.Sub(metrics.StartTime)

	fmt.Print("\n" + strings.Repeat("=", 60) + "\n")
	fmt.Print("🕷️  CRAWL PERFORMANCE METRICS\n")
	fmt.Print(strings.Repeat("=", 60) + "\n")

	// timing information
	fmt.Printf("⏱️  Total Duration: %v\n", duration.Round(time.Second))
	fmt.Printf("🚀 Crawl Rate: %.2f sites/second\n", float64(metrics.SuccessfullyCrawled)/duration.Seconds())

	// success metrics
	fmt.Printf("\n📊 SUCCESS METRICS:\n")
	fmt.Printf("✅ Successfully Crawled: %d\n", atomic.LoadInt64(&metrics.SuccessfullyCrawled))
	fmt.Printf("📈 Success Rate: %.1f%%\n", float64(atomic.LoadInt64(&metrics.SuccessfullyCrawled))/float64(atomic.LoadInt64(&metrics.TotalProcessed))*100)

	// error breakdown
	fmt.Printf("\n❌ ERROR BREAKDOWN:\n")
	fmt.Printf("🤖 Blocked by robots.txt: %d\n", atomic.LoadInt64(&metrics.BlockedByRobots))
	fmt.Printf("💥 Failed Crawls: %d\n", atomic.LoadInt64(&metrics.FailedCrawls))
	fmt.Printf("📄 Empty Content: %d\n", atomic.LoadInt64(&metrics.EmptyContent))
	fmt.Printf("🧠 Embedding Errors: %d\n", atomic.LoadInt64(&metrics.EmbeddingErrors))
	fmt.Printf("🗄️  MongoDB Errors: %d\n", atomic.LoadInt64(&metrics.MongoErrors))

	// totals
	fmt.Printf("\n📋 TOTALS:\n")
	fmt.Printf("🔢 Total Processed: %d\n", atomic.LoadInt64(&metrics.TotalProcessed))
	fmt.Printf("⏰ Average Time per Site: %v\n", duration/time.Duration(atomic.LoadInt64(&metrics.TotalProcessed)))

	fmt.Print(strings.Repeat("=", 60) + "\n")
}

func main() {
	loadEnv()
	workers := flag.Int("workers", defaultWorkers, "number of concurrent workers")
	search := flag.String("search", "", "semantic search query (when set, crawlers won't run)")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	env, err := utils.InitMongo(ctx)
	if err != nil {
		log.Fatalf("mongo init error: %v", err)
	}
	defer func() {
		_ = env.Client.Disconnect(context.Background())
	}()

	if strings.TrimSpace(*search) != "" {
		if err := utils.PerformSearch(ctx, env, *search); err != nil {
			log.Fatalf("search error: %v", err)
		}
		return
	}

	if env.OpenAIAPIKey == "" {
		log.Fatal("OPENAI_API_KEY is not set")
	}

	// try to fetch robots.txt from multiple sources
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

	// extract episode links from the starting page
	episodeLinks, err := scraper.ExtractEpisodeLinks(ctx, startURL)
	if err != nil {
		log.Fatalf("failed to parse episode links: %v", err)
	}
	if len(episodeLinks) == 0 {
		log.Fatal("no episode links found; check selector or page structure")
	}

	// initialize crawl metrics
	metrics := &CrawlMetrics{
		StartTime: time.Now(),
	}

	// set up worker pool with buffered channel
	jobs := make(chan string, len(episodeLinks))
	var wg sync.WaitGroup

	workerCount := *workers
	if workerCount < 1 {
		workerCount = 1
	}
	log.Printf("starting %d workers…", workerCount)

	// start worker goroutines
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go worker(ctx, i+1, env, robots, jobs, &wg, metrics)
	}

	// send jobs to workers
	for _, link := range episodeLinks {
		jobs <- link
	}
	close(jobs) // close channel to signal no more jobs

	// wait for all workers to finish
	wg.Wait()
	log.Println("crawl completed.")

	// display performance metrics
	displayMetrics(metrics)
}
