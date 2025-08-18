package utils

import (
	"encoding/csv"
	"fmt"
	"os"
)

type Product struct {
	Name     string
	Price    string
	ImageURL string
}

// function to export scraped data to CSV
func ExportToCSV(filename string, products []Product) {
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
