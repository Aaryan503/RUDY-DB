package main

import (
	"bufio"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
)

var db Database

func main() {
	var i int
	wal, err := os.OpenFile(
		"wal.log",
		os.O_CREATE|os.O_RDWR|os.O_APPEND,
		0644,
	)
	if err != nil {
		panic(err)
	}
	db = Database{
		tables:       make(map[string]*Table),
		walFile:      wal,
		snapshotChan: make(chan struct{}, 1),
	}
	err = db.loadSnapshot()
	if err != nil {
		panic(err)
	}
	err = db.loadWAL()
	if err != nil {
		panic(err)
	}
	go db.snapshotWorker()
	fmt.Println("Welcome to RUDYDB, a simple, recoverable and concurrent DB implementation in Go.\nDo you want to run a CLI or an API?\nEnter 1 for API, 2 for CLI: ")
	fmt.Scanln(&i)
	if i == 1 {
		r := chi.NewRouter()
		r.Post("/query", handleQuery)
		r.Post("/tables/{name}", createTable)
		r.Post("/tables/{tableName}/rows", insertRow)
		r.Delete("/tables/{name}", deleteTable)
		r.Delete("/tables/{tableName}/row/{rowId}", deleteRow)
		r.Get("/tables", getTables)
		r.Get("/tables/{tableName}", getTable)
		r.Get("/tables/{tableName}/rows/{rowId}", getRow)
		r.Put("/tables/{tableName}/rows/{rowId}", updateRow)
		fmt.Println("Server running on :8080")
		err = http.ListenAndServe(":8080", r)
		if err != nil {
			panic(err)
		}
	} else if i == 2 {
		exec := newExecutor(&db)
		scanner := bufio.NewScanner(os.Stdin)
		for {
			fmt.Print("rudydb> ")
			if !scanner.Scan() {
				break
			}
			query := strings.TrimSpace(scanner.Text())
			if query == "" {
				continue
			}
			result, err := exec.execute(query)
			if err != nil {
				fmt.Println("Error:", err)
				continue
			}
			switch v := result.(type) {
			case *SelectResult:
				if len(v.Columns) == 0 {
					fmt.Println("No columns selected")
					break
				}
				for _, col := range v.Columns {
					fmt.Printf("%-15s", col)
				}
				fmt.Println()
				for range v.Columns {
					fmt.Printf("%-15s", "--------------")
				}
				fmt.Println()
				for _, row := range v.Rows {
					for _, col := range v.Columns {
						fmt.Printf("%-15v", row[col])
					}
					fmt.Println()
				}
				fmt.Printf("\n%d row(s)\n", len(v.Rows))

			case Row:
				fmt.Println("Inserted:")
				for k, val := range v {
					fmt.Printf("  %s = %v\n", k, val)
				}

			case map[string]int:
				for k, val := range v {
					fmt.Printf("%s: %d\n", k, val)
				}

			case *Table:
				if v == nil {
					fmt.Println("Done")
				} else {
					fmt.Printf("Table: %s (%d columns)\n", v.Name, len(v.Columns))
				}
			default:
				fmt.Printf("%+v\n", v)
			}
		}
	}

}
