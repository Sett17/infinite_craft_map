package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/sirupsen/logrus"
)

type ApiResponse struct {
	Result string `json:"result"`
	Emoji  string `json:"emoji"`
	IsNew  bool   `json:"isNew"`
}

const dbName = "./items.db"
const apiURL = "https://neal.fun/api/infinite-craft/pair"

var localItemsCache map[string]string

func main() {
	logrus.SetLevel(logrus.DebugLevel)
	db := initializeDatabase()
	defer db.Close()

	initializeLocalCache(db)

	N := 500000
	exploreCombinations(db, N, N*5)
}

func initializeLocalCache(db *sql.DB) {
	localItemsCache = make(map[string]string)
	rows, err := db.Query("SELECT name, emoji FROM items")
	if err != nil {
		logrus.Fatal("Failed to initialize local cache: ", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name, emoji string
		if err := rows.Scan(&name, &emoji); err != nil {
			logrus.Fatal("Failed to read item for local cache: ", err)
		}
		localItemsCache[name] = emoji
	}
	logrus.Info("Local cache initialized with items from database")
}

func initializeDatabase() *sql.DB {
	dbExists := checkDatabaseExists()

	logrus.Debug("Database exists: ", dbExists)
	db, err := sql.Open("sqlite3", dbName)
	if err != nil {
		logrus.Fatal("Failed to open database: ", err)
	}

	if !dbExists {
		createTables(db)
		insertInitialItems(db)
	}
	return db
}

func checkDatabaseExists() bool {
	if _, err := os.Stat(dbName); err != nil {
		return !os.IsNotExist(err)
	}
	return true
}

func createTables(db *sql.DB) {
	itemsTableSQL := `
    CREATE TABLE items (
        name TEXT PRIMARY KEY,
        emoji TEXT NOT NULL,
        isNew BOOLEAN NOT NULL
    );`

	combinationsTableSQL := `
    CREATE TABLE combinations (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        firstItem TEXT NOT NULL,
        secondItem TEXT NOT NULL,
        resultItem TEXT NOT NULL,
        UNIQUE(firstItem, secondItem),
        FOREIGN KEY (firstItem) REFERENCES items(name),
        FOREIGN KEY (secondItem) REFERENCES items(name),
        FOREIGN KEY (resultItem) REFERENCES items(name)
    );`

	_, err := db.Exec(itemsTableSQL)
	if err != nil {
		logrus.Fatal("Failed to create items table: ", err)
	}
	logrus.Info("Created items table")

	_, err = db.Exec(combinationsTableSQL)
	if err != nil {
		logrus.Fatal("Failed to create combinations table: ", err)
	}
	logrus.Info("Created combinations table")
}

func insertInitialItems(db *sql.DB) {
	initialItems := []struct {
		Name  string
		Emoji string
	}{
		{"Water", "💧"},
		{"Fire", "🔥"},
		{"Wind", "🌬️"},
		{"Earth", "🌍"},
	}

	for _, item := range initialItems {
		_, err := db.Exec("INSERT INTO items (name, emoji, isNew) VALUES (?, ?, ?)", item.Name, item.Emoji, false)
		if err != nil {
			logrus.Fatal("Failed to insert initial items: ", err)
		}

	}
	logrus.Info("Inserted initial items")
}

func combineElements(first, second string, db *sql.DB) {
	response, err := callApi(first, second)
	if err != nil {
		logrus.Fatal("Failed to call API: ", err)
	}

	insertOrUpdateItem(response.Result, response.Emoji, response.IsNew, db)
	insertCombination(first, second, response.Result, db)
}

func callApi(first, second string) (*ApiResponse, error) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	q := req.URL.Query()
	q.Add("first", first)
	q.Add("second", second)
	req.URL.RawQuery = q.Encode()

	req.Header.Add("referer", "https://neal.fun/infinite-craft/")
	req.Header.Add("user-agent", "InfiniteCraft_Mapper/rate-limited")

	logrus.Debug("Calling API with URL: ", req.URL.String())

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter, err := strconv.Atoi(resp.Header.Get("Retry-After"))
		if err != nil {
			retryAfter = 60 // Default to 60 seconds if not parseable
		}
		time.Sleep(time.Duration(retryAfter+1) * time.Second)
		return callApi(first, second) // Recursively retry the request
	} else if resp.StatusCode >= 400 {
		panic(fmt.Sprintf("API request failed with status code: %d", resp.StatusCode))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var response ApiResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	return &response, nil
}

func insertOrUpdateItem(name, emoji string, isNew bool, db *sql.DB) {
	logrus.Debugf("Inserting or updating item: %s, %s, %t", name, emoji, isNew)
	localItemsCache[name] = emoji // Update local cache
	_, err := db.Exec("INSERT INTO items (name, emoji, isNew) VALUES (?, ?, ?) ON CONFLICT(name) DO UPDATE SET emoji=excluded.emoji, isNew=excluded.isNew", name, emoji, isNew)
	if err != nil {
		logrus.Fatal("Failed to insert or update item: ", err)
	}
}

func insertCombination(firstItem, secondItem, resultItem string, db *sql.DB) {
	logrus.Debugf("Inserting combination: %s, %s, %s", firstItem, secondItem, resultItem)
	_, err := db.Exec("INSERT INTO combinations (firstItem, secondItem, resultItem) VALUES (?, ?, ?)", firstItem, secondItem, resultItem)
	if err != nil {
		logrus.Fatal("Failed to insert combination: ", err)
	}
}

func getRandomItems() (string, string, error) {
	var items []string
	for item := range localItemsCache {
		items = append(items, item)
	}

	if len(items) < 2 {
		return "", "", fmt.Errorf("not enough items to combine")
	}

	firstIndex := rand.Intn(len(items))
	secondIndex := firstIndex
	for secondIndex == firstIndex {
		secondIndex = rand.Intn(len(items))
	}

	return items[firstIndex], items[secondIndex], nil
}

// Function to check if a combination has already been attempted
func combinationExists(firstItem, secondItem string, db *sql.DB) (bool, error) {
	query := `SELECT COUNT(*) FROM combinations WHERE firstItem = ? AND secondItem = ?`
	var count int
	err := db.QueryRow(query, firstItem, secondItem).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// Main exploration function to randomly try new combinations
func exploreCombinations(db *sql.DB, maxCombinations, maxAttempts int) {
	attempts := 0
	createdCombinations := 0

	for createdCombinations < maxCombinations && attempts < maxAttempts {
		firstItem, secondItem, err := getRandomItems()
		if err != nil {
			logrus.Error("Error getting random items: ", err)
			return
		}

		exists, err := combinationExists(firstItem, secondItem, db)
		if err != nil {
			logrus.Error("Error checking if combination exists: ", err)
			return
		}

		if !exists {
			combineElements(firstItem, secondItem, db)
			createdCombinations++
		}

		attempts++

		time.Sleep(time.Millisecond * 50)
	}

	logrus.Info("Finished creating combinations. Total created: ", createdCombinations, ", Total attempts: ", attempts)
}
