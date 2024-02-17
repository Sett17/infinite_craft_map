package main

import (
	"bytes"
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"

	_ "github.com/mattn/go-sqlite3"
)

var (
	templates *template.Template
	db        *sql.DB
)

func main() {
	initDB("items.db")
	defer db.Close()
	templates = template.Must(template.New("").ParseGlob("templates/*.html"))

	mux := http.NewServeMux()

	logMux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s\n", r.Method, r.URL.Path)
		mux.ServeHTTP(w, r)
	})

	mux.HandleFunc("/", serveStartPage)
	mux.HandleFunc("/search", handleSearch)
	mux.HandleFunc("/count", handleItemCount)
	mux.HandleFunc("/i/{name}", handleItem)

	log.Println("Server started on :8080")
	http.ListenAndServe(":8080", logMux)
}

func serveStartPage(w http.ResponseWriter, r *http.Request) {
	log.Println("Serving start page")
	totalItems, _ := getTotalItemCount()
	_ = totalItems
	data := struct {
		Title      string
		TotalItems int
		MaybeItem  string
	}{Title: "Infinite Craft Search", TotalItems: totalItems, MaybeItem: ""}
	if err := templates.ExecuteTemplate(w, "start.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleSearch(w http.ResponseWriter, r *http.Request) {
	searchQuery := r.FormValue("item")
	log.Printf("Handling search for query: '%s'", searchQuery)

	items, limited, err := searchItems(searchQuery)
	if err != nil {
		log.Printf("Error fetching items: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}


		Items   []Item
		Limited bool
	}{Items: items, Limited: limited})
	if err != nil {
		log.Printf("Error executing template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func handleItemCount(w http.ResponseWriter, r *http.Request) {
	count, err := getTotalItemCount()
	if err != nil {
		http.Error(w, "Failed to get item count", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%d", count)
}

func handleItem(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	item, err := getItem(name)
	if err != nil {
		log.Printf("Error fetching item: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if item == nil {
		log.Printf("Item not found: %s", name)
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	combinations, err := getCombinations(item)
	if err != nil {
		log.Printf("Error fetching combinations: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	tempWriter := &bytes.Buffer{}
	err = templates.ExecuteTemplate(tempWriter, "item.html", struct {
		Item         *Item
		Combinations []Combination
	}{Item: item, Combinations: combinations})
	if err != nil {
		log.Printf("Error executing template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	itemHTML := template.HTML(tempWriter.String())

	totalItems, _ := getTotalItemCount()

	err = templates.ExecuteTemplate(w, "start.html", struct {
		Title      string
		TotalItems int
		MaybeItem  template.HTML
	}{Title: fmt.Sprintf("%s | Infinite Craft Search", item.Name), TotalItems: totalItems, MaybeItem: itemHTML})
}

func getItem(name string) (*Item, error) {
	var item Item
	stmt, err := db.Prepare(`SELECT name, emoji, isNew FROM items WHERE name = ?`)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	row := stmt.QueryRow(name)
	if err := row.Scan(&item.Name, &item.Emoji, &item.IsNew); err != nil {
		return nil, err
	}

	return &item, nil
}

func getCombinations(item *Item) ([]Combination, error) {
	stmt, err := db.Prepare(`SELECT
	A.name AS firstName,
	A.emoji AS firstEmoji,
	B.name AS secondName,
	B.emoji AS secondEmoji
FROM
	combinations
JOIN
	items A ON combinations.firstItem = A.name
JOIN
	items B ON combinations.secondItem = B.name
WHERE
	combinations.resultItem = ?`)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	rows, err := stmt.Query(item.Name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	combinations := make([]Combination, 0)

	for rows.Next() {
		combination := Combination{
			Item1:  &Item{},
			Item2:  &Item{},
			Result: item,
		}
		if err := rows.Scan(&combination.Item1.Name, &combination.Item1.Emoji, &combination.Item2.Name, &combination.Item2.Emoji); err != nil {
			return nil, err
		}
		log.Printf("Combination: %v", combination)
		combinations = append(combinations, combination)
	}

	return combinations, nil
}

func initDB(dataSourceName string) {
	var err error
	db, err = sql.Open("sqlite3", dataSourceName)
	if err != nil {
		log.Fatal(err)
	}
	if err = db.Ping(); err != nil {
		log.Fatal(err)
	}
}

type Item struct {
	Name  string
	Emoji string
	IsNew bool
}

type Combination struct {
	Item1  *Item
	Item2  *Item
	Result *Item
}

func searchItems(query string) ([]Item, bool, error) {

	limit := 1000
	var items []Item
	stmt, err := db.Prepare(`SELECT name, emoji, isNew FROM items WHERE name LIKE ? LIMIT ?`)
	if err != nil {
		return nil, false, err
	}
	defer stmt.Close()

	rows, err := stmt.Query("%"+query+"%", limit)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	for rows.Next() {
		var item Item
		if err := rows.Scan(&item.Name, &item.Emoji, &item.IsNew); err != nil {
			return nil, false, err
		}
		items = append(items, item)
	}

	return items, len(items) == limit, nil
}

func getTotalItemCount() (int, error) {
	var count int
	row := db.QueryRow(`SELECT COUNT(*) FROM items`)
	err := row.Scan(&count)
	return count, err
}
