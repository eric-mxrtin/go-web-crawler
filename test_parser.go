package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

func main() {
	url := "https://modernfamily.fandom.com/wiki/The_Incident" // example page
	paragraphs, err := extractEpisodeSummary(url)
	if err != nil {
		log.Fatal(err)
	}

	for i, p := range paragraphs {
		fmt.Printf("%d: %s\n\n", i+1, p)
	}
}

// extractEpisodeSummary fetches a page and extracts the episode summary paragraphs
func extractEpisodeSummary(url string) ([]string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status code error: %d %s", resp.StatusCode, resp.Status)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	// Try first approach: Plot -> Main_cast
	paragraphs := getParagraphsBetweenHeaders(doc, "Plot", "Main_cast")

	// If empty, fallback to Episode_Description -> Main_Cast
	if len(paragraphs) == 0 {
		paragraphs = getParagraphsBetweenHeaders(doc, "Episode_Description", "Main_Cast")
	}

	return paragraphs, nil
}

// getParagraphsBetweenHeaders extracts <p> text between two h2 headers with spans of given IDs
func getParagraphsBetweenHeaders(doc *goquery.Document, startID, endID string) []string {
	var paragraphs []string
	startFound := false
	stopFound := false

	doc.Find("h2, p").Each(func(i int, s *goquery.Selection) {
		if stopFound {
			return
		}

		if goquery.NodeName(s) == "h2" {
			span := s.Find("span.mw-headline")
			if span.Length() > 0 {
				id := span.AttrOr("id", "")
				if id == startID {
					startFound = true
					return
				}
				if id == endID {
					stopFound = true
					return
				}
			}
		}

		if startFound && !stopFound && goquery.NodeName(s) == "p" {
			text := strings.TrimSpace(s.Text())
			if text != "" {
				paragraphs = append(paragraphs, text)
			}
		}
	})

	return paragraphs
}
