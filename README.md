# Modern Family Web Crawler 🕷️

A production-ready web crawler that scrapes Modern Family episode data from Fandom Wiki, generates semantic embeddings using OpenAI's API, and stores everything in MongoDB for vector-based semantic search.

## 🏗️ Architecture Overview

<img width="1054" height="667" alt="image" src="https://github.com/user-attachments/assets/7efd139d-4360-469c-b28c-482071870a98" />
<br/>
<img width="1025" height="631" alt="image" src="https://github.com/user-attachments/assets/837e3fed-0be9-4117-9c39-a450583b1d08" />
<br/>
<img width="988" height="363" alt="image" src="https://github.com/user-attachments/assets/c5ec44b6-ba7a-49c6-911b-2d9046444c7a" />

### 🔄 Concurrency Model

The crawler implements a worker pool pattern using Go's goroutines and channels:

```go
// Worker pool with buffered channel
jobs := make(chan string, len(episodeLinks))
var wg sync.WaitGroup

// Start worker goroutines
for i := 0; i < workerCount; i++ {
    wg.Add(1)
    go worker(ctx, i+1, env, robots, jobs, &wg)
}
```

Each worker goroutine:
- Processes episode URLs from a shared job channel
- Fetches page content with retry logic
- Generates embeddings using OpenAI API
- Stores data in MongoDB with upsert operations

### 🔍 Link Traversal

The crawler is currently targeted, meaning:

1. All URls are extracted from a seed URL before crawling
2. No link discovery during crawling

### 🤖 Robots.txt Compliance

The crawler respects robots.txt rules by:
- Checking each URL against robots.txt before crawling
- Using a proper user-agent string
- Implementing rate limiting between requests

### 🔄 Retry Logic & Error Handling

Robust error handling with exponential backoff:
- HTTP requests retry up to 3 times with increasing delays
- OpenAI API calls handle token limits gracefully
- MongoDB operations use upsert to handle duplicates

### 🧠 Semantic Search with Vector Embeddings

- Uses OpenAI's `text-embedding-3-large` model (3072 dimensions)
- Stores embeddings in MongoDB with vector search index to support NLQ's

### 🔍 MongoDB Vector Search

Uses MongoDB Atlas vector search for semantic similarity:
- Creates vector search index with cosine similarity
- Supports 3072-dimensional embeddings
- Returns top 10 most relevant results
- Displays results with color-coded scores and previews

## 🚀 Usage

### Setup

1. **Install dependencies:**
```bash
go mod tidy
```

2. **Set environment variables:**
```bash
export MONGO_URI="mongodb+srv://..."
export OPENAI_API_KEY="sk-..."
export MONGO_DB="modern_family" # optional, defaults to modern_family
```

3. **Create MongoDB vector search index:**
```json
{
  "mappings": {
    "dynamic": false,
    "fields": {
      "embedding": {
        "type": "knnVector",
        "dimensions": 3072,
        "similarity": "cosine"
      }
    }
  }
}
```

### Crawling Episodes

```bash
# Crawl all episodes with default 6 workers
go run main.go

# Crawl with custom worker count
go run main.go -workers 10
```

### Semantic Search

```bash
# Search for episodes where Phil gets hurt
go run main.go -search "Phil get's hurt"

# Search for episodes where the family is on vacation
go run main.go -search "The Dunphy family is on vacation"
```

## 📁 Project Structure

```
├── main.go                 # Main application entry point
├── scraper/
│   ├── links.go           # Episode link extraction with BFS
│   └── plot.go            # Episode content scraping
├── utils/
│   ├── embeddings.go      # OpenAI API integration
│   └── mongodb.go         # Database operations and vector search
├── go.mod                 # Go module dependencies
└── README.md              # This file
```

## 🚀 Next Steps: AWS & System Design

To convert this crawler from a targeted crawler to a performant web crawler (for a much larger set of URLs), I would implement these changes:

### 🏗️ AWS Infrastructure Migration

**Queue Management:**
- **SQS Integration**: Replace in-memory channels with AWS SQS for job distribution
- **Dead Letter Queue (DLQ)**: Implement retry logic with exponential backoff for failed messages
- **Message Visibility**: Handle long-running tasks with proper visibility timeouts

### 🔄 Enhanced Crawling Architecture

**Separation of Concerns:**
- **Crawler Service**: Dedicated service for fetching and storing webpages
- **Parsing Worker**: Separate service for HTML processing and content extraction
- **Rate Limiter**: Centralized rate limiting service with domain-specific rules

### 📊 Monitoring & Observability

**Performance Metrics:**
- Crawl success rates and error tracking
- Queue depth and processing times
- Storage utilization and costs
- API rate limit compliance

Thanks for reading!
