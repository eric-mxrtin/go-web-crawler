package main

import (
	"encoding/csv"
	"fmt"
	"os"

	"github.com/gocolly/colly"
)

// map that stores visited URls
var visitedUrls = make(map[string]bool)

type Product struct {
	Name     string
	Price    string
	ImageURL string
}

var products []Product

func main() {
	seedUrl := "https://www.scrapingcourse.com/ecommerce/"
	crawl(seedUrl, 0)
}

// function to export scraped data to CSV
func exportToCSV(filename string) {
	// open a CSV file
	file, err := os.Create(filename)
	if err != nil {
		fmt.Println("Error creating CSV file:", err)
		return
	}
	defer file.Close()
	// initialize a writer class
	writer := csv.NewWriter(file)
	defer writer.Flush()

	// write the header row
	writer.Write([]string{"Name", "Price", "Image URL"})

	// write the product details
	for _, product := range products {
		writer.Write([]string{product.Name, product.Price, product.ImageURL})
	}
	fmt.Println("Product details exported to", filename)
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

		products = append(products, Product{
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
		exportToCSV("products.csv")
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
