# modern family web crawler

a production-ready web crawler that scrapes modern family episode data from fandom wiki, generates semantic embeddings using openai's api, and stores everything in mongodb for vector-based semantic search.

## architecture overview

this crawler uses a multi-layered architecture designed for scalability and maintainability:

### core components

- **main.go**: orchestrates the entire crawling process with go concurrency patterns
- **scraper/**: handles web scraping and data extraction
- **utils/**: contains mongodb operations and openai embedding generation
- **mongodb**: stores episode data with vector embeddings for semantic search

### concurrency model

the crawler implements a worker pool pattern using go's goroutines and channels:

```go
// worker pool with buffered channel
jobs := make(chan string, len(episodeLinks))
var wg sync.WaitGroup

// start worker goroutines
for i := 0; i < workerCount; i++ {
    wg.Add(1)
    go worker(ctx, i+1, env, robots, jobs, &wg)
}
```

each worker goroutine:
- processes episode urls from a shared job channel
- fetches page content with retry logic
- generates embeddings using openai api
- stores data in mongodb with upsert operations

### bfs-like link traversal

the scraper uses a breadth-first search approach to discover episode links:

1. starts from the pilot episode page
2. traverses episode tables in the wiki structure
3. extracts links using css selectors
4. resolves relative urls to absolute urls
5. limits traversal to prevent infinite loops

```go
// traverse episode tables using bfs-like approach
doc.Find(tableSelector).EachWithBreak(func(i int, table *goquery.Selection) bool {
    if tableCount >= 3 {
        return false // stop after some tables to avoid too many episodes
    }
    // ... extract links from table
})
```

### robots.txt compliance

the crawler respects robots.txt rules by:

- fetching robots.txt from multiple sources (fandom.com, modernfamily.fandom.com)
- checking each url against robots.txt before crawling
- using a proper user-agent string
- implementing rate limiting between requests

```go
// check if url is allowed by robots.txt for our user-agent
func isAllowed(robots *robotstxt.RobotsData, rawURL string) bool {
    // ... robots.txt validation logic
}
```

### retry logic and error handling

robust error handling with exponential backoff:

- http requests retry up to 3 times with increasing delays
- openai api calls handle token limits gracefully
- mongodb operations use upsert to handle duplicates
- comprehensive logging for debugging

```go
// http request with exponential backoff retry logic
func doRequestWithRetry(req *http.Request) (*http.Response, error) {
    for attempt := 1; attempt <= maxRetries; attempt++ {
        resp, err = httpClient.Do(req)
        if err == nil && resp.StatusCode >= 200 && resp.StatusCode <= 299 {
            return resp, nil
        }
        if attempt < maxRetries {
            time.Sleep(backoff)
            backoff *= 2 // exponential backoff
        }
    }
}
```

### semantic search with vector embeddings

the crawler generates high-quality embeddings for semantic search:

- uses openai's `text-embedding-3-large` model (3072 dimensions)
- normalizes vectors using l2 normalization
- stores embeddings in mongodb with vector search index
- supports natural language queries like "jay's birthday" or "phil's real estate"

```go
// generate embedding vector for text using openai api
func GetEmbedding(ctx context.Context, apiKey, text string) ([]float64, error) {
    // ... openai api integration
}
```

### mongodb vector search

uses mongodb atlas vector search for semantic similarity:

- creates vector search index with cosine similarity
- supports 3072-dimensional embeddings
- returns top 10 most relevant results
- displays results with color-coded scores and previews

```go
// mongodb atlas vector search pipeline
pipeline := mongo.Pipeline{
    {{
        Key: "$vectorSearch",
        Value: bson.D{
            {Key: "index", Value: env.VectorIndex},
            {Key: "path", Value: "embedding"},
            {Key: "queryVector", Value: vec},
            {Key: "numCandidates", Value: 200},
            {Key: "limit", Value: 10},
        },
    }},
}
```

## usage

### setup

1. install dependencies:
```bash
go mod tidy
```

2. set environment variables:
```bash
export MONGO_URI="mongodb+srv://..."
export OPENAI_API_KEY="sk-..."
export MONGO_DB="modern_family" # optional, defaults to modern_family
```

3. create mongodb vector search index:
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

### crawling episodes

```bash
# crawl all episodes with default 6 workers
go run main.go

# crawl with custom worker count
go run main.go -workers 10
```

### semantic search

```bash
# search for episodes about jay's birthday
go run main.go -search "jay's birthday"

# search for episodes about phil's real estate
go run main.go -search "phil's real estate"
```

## technical features

### go concurrency patterns
- worker pool with buffered channels
- waitgroups for goroutine synchronization
- context-based cancellation and timeouts
- channel-based job distribution

### web scraping
- goquery for html parsing
- robots.txt compliance checking
- user-agent spoofing
- rate limiting and retry logic

### data processing
- text cleaning and normalization
- utf-8 validation
- control character removal
- content truncation for api limits

### vector search
- openai embedding generation
- l2 vector normalization
- mongodb atlas vector search
- cosine similarity scoring

### error handling
- exponential backoff retry
- graceful degradation
- comprehensive logging
- resource cleanup

## project structure

```
├── main.go                 # main application entry point
├── scraper/
│   ├── links.go           # episode link extraction with bfs
│   └── plot.go            # episode content scraping
├── utils/
│   ├── embeddings.go      # openai api integration
│   └── mongodb.go         # database operations and vector search
├── go.mod                 # go module dependencies
└── README.md              # this file
```

## performance considerations

- concurrent workers process multiple episodes simultaneously
- buffered channels prevent blocking on job distribution
- mongodb upsert operations handle duplicate episodes efficiently
- vector search uses mongodb atlas for optimal performance
- exponential backoff prevents api rate limiting

## future improvements

- implement content chunking for very long episodes
- add caching layer for frequently accessed episodes
- support for different embedding models
- web interface for search results
- episode recommendation system based on similarity

## dependencies

- go 1.21+
- mongodb atlas (for vector search)
- openai api (for embeddings)
- modern family fandom wiki (data source)

this crawler demonstrates production-ready go development with proper concurrency patterns, error handling, and integration with modern ai services for semantic search capabilities.