package scraper

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// extract episode title and plot summary from wiki page
func ExtractEpisodeData(ctx context.Context, url string) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", "ModernFamilyCrawler/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", "", fmt.Errorf("status code error: %d %s", resp.StatusCode, resp.Status)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", "", err
	}

	// Get title (remove " | Modern Family Wiki | Fandom" suffix)
	title := strings.TrimSuffix(doc.Find("title").Text(), " | Modern Family Wiki | Fandom")

	// Get summary paragraphs
	paragraphs := getParagraphsBetweenHeaders(doc, "Plot", "Main_cast")
	if len(paragraphs) == 0 {
		paragraphs = getParagraphsBetweenHeaders(doc, "Episode_Description", "Main_Cast")
	}

	// Join paragraphs into single summary text
	summary := strings.Join(paragraphs, "\n\n")
	return title, summary, nil
}

// getParagraphsBetweenHeaders remains unchanged from your original implementation
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
