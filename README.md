# Modern Family Web Crawler 🕷️

A production-ready web crawler that scrapes Modern Family episode data from Fandom Wiki, generates semantic embeddings using OpenAI's API, and stores everything in MongoDB for vector-based semantic search.

## 🏗️ Architecture Overview

This crawler uses a multi-layered architecture designed for scalability and maintainability:

### Core Components
- **main.go**: Orchestrates the entire crawling process with Go concurrency patterns
- **scraper/**: Handles web scraping and data extraction
- **utils/**: Contains MongoDB operations and OpenAI embedding generation
- **MongoDB**: Stores episode data with vector embeddings for semantic search

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

### 🔍 BFS-like Link Traversal

The scraper uses a breadth-first search approach to discover episode links:

1. Starts from the pilot episode page
2. Traverses episode tables in the wiki structure
3. Extracts links using CSS selectors
4. Resolves relative URLs to absolute URLs
5. Limits traversal to prevent infinite loops

### 🤖 Robots.txt Compliance

The crawler respects robots.txt rules by:
- Fetching robots.txt from multiple sources
- Checking each URL against robots.txt before crawling
- Using a proper user-agent string
- Implementing rate limiting between requests

### 🔄 Retry Logic & Error Handling

Robust error handling with exponential backoff:
- HTTP requests retry up to 3 times with increasing delays
- OpenAI API calls handle token limits gracefully
- MongoDB operations use upsert to handle duplicates
- Comprehensive logging for debugging

### 🧠 Semantic Search with Vector Embeddings

The crawler generates high-quality embeddings for semantic search:
- Uses OpenAI's `text-embedding-3-large` model (3072 dimensions)
- Normalizes vectors using L2 normalization
- Stores embeddings in MongoDB with vector search index
- Supports natural language queries like "Jay's birthday" or "Phil's real estate"

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
# Search for episodes about Jay's birthday
go run main.go -search "jay's birthday"

# Search for episodes about Phil's real estate
go run main.go -search "phil's real estate"
```

## ⚡ Technical Features

### Go Concurrency Patterns
- Worker pool with buffered channels
- WaitGroups for goroutine synchronization
- Context-based cancellation and timeouts
- Channel-based job distribution

### Web Scraping
- GoQuery for HTML parsing
- Robots.txt compliance checking
- User-agent spoofing
- Rate limiting and retry logic

### Data Processing
- Text cleaning and normalization
- UTF-8 validation
- Control character removal
- Content truncation for API limits

### Vector Search
- OpenAI embedding generation
- L2 vector normalization
- MongoDB Atlas vector search
- Cosine similarity scoring

### Error Handling
- Exponential backoff retry
- Graceful degradation
- Comprehensive logging
- Resource cleanup

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

Based on the system architecture diagram, here are the planned improvements to scale this crawler:

### 🏗️ AWS Infrastructure Migration

**Queue Management:**
- **SQS Integration**: Replace in-memory channels with AWS SQS for job distribution
- **Dead Letter Queue (DLQ)**: Implement retry logic with exponential backoff for failed messages
- **Message Visibility**: Handle long-running tasks with proper visibility timeouts

**Storage Layer:**
- **S3 HTML Data**: Store raw HTML content in S3 for scalable storage
- **S3 Parsed Text**: Save processed text content separately for faster access
- **S3 Lifecycle Policies**: Implement data retention and archival strategies

**Metadata Management:**
- **URL Metadata Table**: Track URL status, S3 links, and last crawl times
- **Domain Metadata**: Store domain-specific information including robots.txt rules
- **DynamoDB Integration**: Use DynamoDB for fast metadata lookups

### 🔄 Enhanced Crawling Architecture

**Separation of Concerns:**
- **Crawler Service**: Dedicated service for fetching and storing webpages
- **Parsing Worker**: Separate service for HTML processing and content extraction
- **Rate Limiter**: Centralized rate limiting service with domain-specific rules

**DNS & Network:**
- **DNS Resolution**: Implement proper DNS caching and resolution
- **Load Balancing**: Distribute crawling load across multiple instances
- **CDN Integration**: Use CloudFront for static content delivery

### 📊 Monitoring & Observability

**AWS Services:**
- **CloudWatch**: Comprehensive logging and metrics
- **X-Ray**: Distributed tracing for request flows
- **SNS/SQS**: Alerting and notification systems

**Performance Metrics:**
- Crawl success rates and error tracking
- Queue depth and processing times
- Storage utilization and costs
- API rate limit compliance

### 🔐 Security & Compliance

**Access Control:**
- **IAM Roles**: Proper service-to-service authentication
- **VPC Configuration**: Network isolation and security groups
- **Secrets Management**: AWS Secrets Manager for API keys

**Compliance Features:**
- **Robots.txt Caching**: Efficient robots.txt rule storage and retrieval
- **Rate Limiting**: Per-domain rate limiting with backoff strategies
- **Data Privacy**: GDPR compliance for user data handling

### 🎯 Scalability Improvements

**Horizontal Scaling:**
- **Auto Scaling Groups**: Dynamic scaling based on queue depth
- **Multi-Region Deployment**: Geographic distribution for global crawling
- **Container Orchestration**: ECS or EKS for containerized services

**Performance Optimization:**
- **Connection Pooling**: Efficient database and HTTP connections
- **Caching Layer**: Redis/ElastiCache for frequently accessed data
- **Batch Processing**: Process multiple URLs in batches for efficiency

This roadmap transforms the current single-process crawler into a distributed, cloud-native system capable of handling large-scale web crawling operations with proper monitoring, security, and compliance features.