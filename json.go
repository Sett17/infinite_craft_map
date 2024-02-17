package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

type Item struct {
	Text       string `json:"text"`
	Emoji      string `json:"emoji"`
	Discovered bool   `json:"discovered"`
}

type ItemsList struct {
	Elements []Item `json:"elements"`
}

func main() {
	// Open the SQLite database
	db, err := sql.Open("sqlite3", "items.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Query the items table
	rows, err := db.Query("SELECT name, emoji, isNew FROM items")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	var itemsList ItemsList
	for rows.Next() {
		var item Item
		err = rows.Scan(&item.Text, &item.Emoji, &item.Discovered)
		if err != nil {
			log.Fatal(err)
		}
		itemsList.Elements = append(itemsList.Elements, item)
	}

	if err = rows.Err(); err != nil {
		log.Fatal(err)
	}

	// Marshal data into minified JSON
	jsonData, err := json.Marshal(itemsList)
	if err != nil {
		log.Fatal(err)
	}

	// Save minified JSON to file
	err = os.WriteFile("localStorage.json", jsonData, 0644)
	if err != nil {
		log.Fatal("Error writing to file:", err)
	}

	// Optionally print to stdout as confirmation or for debugging
	fmt.Printf("Minified JSON data saved to localStorage.json. %d items found", len(itemsList.Elements))
}
