package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

func main() {
	url := "https://modernfamily.fandom.com/wiki/Pilot" // example page

	// Fetch page
	resp, err := http.Get(url)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Fatalf("failed to fetch page: %d %s", resp.StatusCode, resp.Status)
	}

	// Load HTML into goquery
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	// Selector for the tables (either spelling of colors)
	tableSelector := "table.toccolors.collapsible.collapsed, table.toccolours.collapsible.collapsed"

	// Find first 10 tables
	count := 0
	doc.Find(tableSelector).EachWithBreak(func(i int, table *goquery.Selection) bool {
		if count >= 12 {
			return false // stop after first 10
		}
		count++

		// Go to tbody
		tbody := table.Find("tbody")
		if tbody.Length() == 0 {
			return true // skip if no tbody
		}

		// Get second tr
		tr := tbody.Find("tr").Eq(1) // zero-indexed
		if tr.Length() == 0 {
			return true // skip if missing
		}

		// Get th inside tr
		th := tr.Find("th")
		if th.Length() == 0 {
			return true // skip if missing
		}

		// Grab all <a> hrefs
		th.Find("a").Each(func(i int, a *goquery.Selection) {
			href, exists := a.Attr("href")
			if !exists || strings.TrimSpace(href) == "" {
				return
			}
			title := a.AttrOr("title", "")
			if strings.HasPrefix(title, "Season ") {
				return // skip season links
			}
			fmt.Println(href)
		})

		return true
	})
}
