package scraper

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// extract episode links from the modern family wiki page using bfs-like traversal
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

	// parse base url for absolute url construction
	baseURL, err := url.Parse(seedURL)
	if err != nil {
		return nil, fmt.Errorf("invalid seed url: %w", err)
	}

	const maxTables = 12 // full is 12
	doc.Find(tableSelector).EachWithBreak(func(i int, table *goquery.Selection) bool {
		if tableCount >= maxTables {
			return false
		}
		tableCount++

		tbody := table.Find("tbody")
		if tbody.Length() == 0 {
			return true // continue to next table
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

			// resolve relative urls to absolute
			hrefURL, err := url.Parse(href)
			if err != nil {
				return
			}

			absURL := baseURL.ResolveReference(hrefURL).String()
			links = append(links, absURL)
		})

		return true // continue processing
	})

	return links, nil
}
