package main

import (
	"crawler/utils"
	"fmt"

	"github.com/gocolly/colly"
)

// map that stores visited URls
var visitedUrls = make(map[string]bool)

type Product struct {
	Name     string
	Price    string
	ImageURL string
}

var products []utils.Product // instead of []Product

func main() {
	seedUrl := "https://www.scrapingcourse.com/ecommerce/"
	crawl(seedUrl, 1)
}

func crawl(currentUrl string, maxDepth int) {
	// create a collector
	c := colly.NewCollector(
		colly.AllowedDomains("www.scrapingcourse.com"),
		colly.MaxDepth(maxDepth),
	)

	// extract + log page title
	c.OnHTML("title", func(e *colly.HTMLElement) {
		fmt.Println("Page Title: ", e.Text)
	})

	// ------ GETTING PRODUCT DETAILS ----- //
	// list item with class product
	c.OnHTML("li.product", func(e *colly.HTMLElement) {

		// retrieve info
		name := e.ChildText(".product-name")
		price := e.ChildText(".product-price")
		imageURL := e.ChildAttr(".product-image", "src")

		products = append(products, utils.Product{
			Name:     name,
			Price:    price,
			ImageURL: imageURL,
		})
	})

	// ------ GETTING PAGINATION LINKS ----- //
	c.OnHTML("a.page-numbers", func(e *colly.HTMLElement) {
		// get absolute URL to avoid crawling relative paths
		link := e.Request.AbsoluteURL(e.Attr("href"))

		// check if its been visited:
		if link != "" && !visitedUrls[link] {
			visitedUrls[link] = true
			fmt.Println("Found Link: ", link)
			// visit this url
			e.Request.Visit(link)
		}
	})

	// before making a request print "Visiting ..."
	c.OnRequest(func(r *colly.Request) {
		fmt.Println("Visiting ", r.URL)
	})

	// define actions after extracting data.
	c.OnScraped(func(r *colly.Response) {
		fmt.Println("Data extraction complete", r.Request.URL)
		// export the collected products to a CSV file after scraping.
		utils.ExportToCSV("products.csv", products)
	})

	// handle request errors
	c.OnError(func(e *colly.Response, err error) {
		fmt.Println("Request URL: ", e.Request.URL, " failed with response: ", e, "\nError: ", err)
	})

	// visit the seed URL
	err := c.Visit(currentUrl)
	if err != nil {
		fmt.Println("Error visiting page: ", err)
	}
}
