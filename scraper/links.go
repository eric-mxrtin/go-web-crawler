package scraper

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

func ExtractEpisodeLinks(ctx context.Context, seedURL string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", seedURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ModernFamilyCrawler/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to fetch page: %d %s", resp.StatusCode, resp.Status)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	var links []string
	tableSelector := "table.toccolors.collapsible.collapsed, table.toccolours.collapsible.collapsed"
	tableCount := 0

	// Parse base URL for proper absolute URL construction
	baseURL, err := url.Parse(seedURL)
	if err != nil {
		return nil, fmt.Errorf("invalid seed URL: %w", err)
	}

	doc.Find(tableSelector).EachWithBreak(func(i int, table *goquery.Selection) bool {
		if tableCount >= 12 {
			return false // Stop after 12 tables
		}
		tableCount++

		tbody := table.Find("tbody")
		if tbody.Length() == 0 {
			return true // Continue to next table
		}

		tr := tbody.Find("tr").Eq(1)
		if tr.Length() == 0 {
			return true
		}

		th := tr.Find("th")
		if th.Length() == 0 {
			return true
		}

		th.Find("a").Each(func(i int, a *goquery.Selection) {
			href, exists := a.Attr("href")
			if !exists || strings.TrimSpace(href) == "" {
				return
			}
			title := a.AttrOr("title", "")
			if strings.HasPrefix(title, "Season ") {
				return
			}

			// Properly resolve the URL
			hrefURL, err := url.Parse(href)
			if err != nil {
				return
			}

			// Create absolute URL using the base URL
			absURL := baseURL.ResolveReference(hrefURL).String()
			links = append(links, absURL)
		})

		return true // Continue processing
	})

	return links, nil
}
