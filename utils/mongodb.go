package utils

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// ANSI color codes for terminal output
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorPurple = "\033[35m"
	ColorCyan   = "\033[36m"
	ColorWhite  = "\033[37m"
	ColorBold   = "\033[1m"
)

// episode document schema for mongodb
type EpisodeDoc struct {
	ID             primitive.ObjectID `bson:"_id,omitempty"`
	URL            string             `bson:"url"`
	PageTitle      string             `bson:"page_title"`
	EpisodeContent string             `bson:"episode_content"`
	Embedding      []float64          `bson:"embedding"` // store as float64 array for BSON
}

// mongodb connection and configuration
type MongoEnv struct {
	Client       *mongo.Client
	DB           *mongo.Database
	Collection   *mongo.Collection
	VectorIndex  string
	OpenAIAPIKey string
}

// initialize mongodb connection and return environment
func InitMongo(ctx context.Context) (*MongoEnv, error) {
	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		return nil, errors.New("MONGO_URI is not set")
	}

	// connect to mongodb
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		return nil, err
	}

	// test connection
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		return nil, err
	}

	dbName := os.Getenv("MONGO_DB")
	if dbName == "" {
		dbName = "modern_family"
	}

	return &MongoEnv{
		Client:       client,
		DB:           client.Database(dbName),
		Collection:   client.Database(dbName).Collection("episodes"),
		VectorIndex:  "vector_index",
		OpenAIAPIKey: os.Getenv("OPENAI_API_KEY"),
	}, nil
}

// save episode to mongodb using upsert (update or insert)
func SaveEpisode(ctx context.Context, env *MongoEnv, doc *EpisodeDoc) error {
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
		options.Update().SetUpsert(true),
	)
	return err
}

// perform vector search using mongodb atlas $vectorSearch
func PerformSearch(ctx context.Context, env *MongoEnv, query string) error {
	if strings.TrimSpace(query) == "" {
		return errors.New("empty query")
	}

	// check if collection has documents
	count, err := env.Collection.CountDocuments(ctx, bson.M{})
	if err != nil {
		return fmt.Errorf("failed to count documents: %w", err)
	}

	if count == 0 {
		return fmt.Errorf("no documents found in collection")
	}

	// generate embedding for search query
	vec, err := GetEmbedding(ctx, env.OpenAIAPIKey, query)
	if err != nil {
		return fmt.Errorf("failed to embed query: %w", err)
	}

	// mongodb atlas vector search pipeline
	pipeline := mongo.Pipeline{
		{{
			Key: "$vectorSearch",
			Value: bson.D{
				{Key: "index", Value: env.VectorIndex},
				{Key: "path", Value: "embedding"},
				{Key: "queryVector", Value: vec},
				{Key: "numCandidates", Value: 200},
				{Key: "limit", Value: 10}, // top 10 results
			},
		}},
		{{Key: "$project", Value: bson.D{
			{Key: "page_title", Value: 1},
			{Key: "url", Value: 1},
			{Key: "episode_content", Value: 1}, // include content for preview
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
		PageTitle      string  `bson:"page_title"`
		URL            string  `bson:"url"`
		EpisodeContent string  `bson:"episode_content"`
		Score          float64 `bson:"score"`
	}

	// Print header
	fmt.Printf("\n%s%s🔍 Search Results for: %s%s\n", ColorBold, ColorCyan, ColorYellow, query)
	fmt.Printf("%s%s%s", ColorCyan, strings.Repeat("=", 60), ColorReset)
	fmt.Printf("\n\n")

	i := 0
	for cur.Next(ctx) {
		i++
		var r res
		if err := cur.Decode(&r); err != nil {
			log.Printf("decode error: %v", err)
			continue
		}

		// Convert score to percentage
		scorePercent := r.Score * 100

		// Get color based on score
		var scoreColor string
		if scorePercent >= 80 {
			scoreColor = ColorGreen
		} else if scorePercent >= 60 {
			scoreColor = ColorYellow
		} else {
			scoreColor = ColorRed
		}

		// Truncate content for preview
		preview := r.EpisodeContent
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}

		// Print formatted result
		fmt.Printf("%s%s%d.%s %s%s%s\n", ColorBold, ColorBlue, i, ColorReset, ColorWhite, r.PageTitle, ColorReset)
		fmt.Printf("   %s%s%s\n", ColorPurple, r.URL, ColorReset)
		fmt.Printf("   %sScore: %s%.1f%%%s\n", ColorCyan, scoreColor, scorePercent, ColorReset)
		fmt.Printf("   %sPreview: %s%s%s\n", ColorCyan, ColorWhite, preview, ColorReset)
		fmt.Printf("\n")
	}

	if i == 0 {
		fmt.Printf("%s%s❌ No results found%s\n", ColorRed, ColorBold, ColorReset)
	} else {
		fmt.Printf("%s%s✅ Found %d results%s\n", ColorGreen, ColorBold, i, ColorReset)
	}
	return nil
}
