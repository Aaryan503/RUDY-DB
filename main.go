package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
)

var db Database

func main() {
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
}
