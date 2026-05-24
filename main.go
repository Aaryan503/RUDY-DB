package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

type Table struct {
	Name    string         `json:"name"`
	Columns []Column       `json:"columns,omitempty"`
	Rows    map[string]Row `json:"rows,omitempty"`
	lock    sync.RWMutex   `json:"-"`
}
type Column struct {
	Name string `json:"name"`
	Type string `json:"type"`
}
type Row map[string]interface{}

type WAL struct {
	OpNumber  int                    `json:"opNumber"`
	Operation string                 `json:"operation"`
	TableName string                 `json:"tableName"`
	RowID     string                 `json:"rowId,omitempty"`
	RowData   map[string]interface{} `json:"rowData,omitempty"`
	Columns   []Column               `json:"columns,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

type Snapshot struct {
	LastOpNumber int               `json:"lastOpNumber"`
	Items        map[string]*Table `json:"items"`
}

type Database struct {
	mu           sync.RWMutex
	tables       map[string]*Table
	walFile      *os.File
	lastOpNumber int
	snapshotChan chan struct{}
}
type CreateTableRequest struct {
	Columns []Column `json:"columns"`
}

var db Database

func validType(t string) bool {
	switch t {
	case "string", "int", "bool":
		return true
	default:
		return false
	}
}

func (db *Database) createTable(name string, columns []Column) (*Table, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	_, exists := db.tables[name]
	if exists {
		return nil, fmt.Errorf("table already exists")
	}
	columnNames := make(map[string]bool)
	for _, c := range columns {
		if c.Name == "" {
			return nil, fmt.Errorf("column name cannot be empty")
		}
		if !validType(c.Type) {
			return nil, fmt.Errorf("unsupported type: %s", c.Type)
		}
		if columnNames[c.Name] {
			return nil, fmt.Errorf("duplicate column: %s", c.Name)
		}
		columnNames[c.Name] = true
	}

	op := WAL{
		OpNumber:  db.lastOpNumber + 1,
		Operation: "CREATE_TABLE",
		TableName: name,
		Columns:   columns,
		Timestamp: time.Now(),
	}
	err := db.appendWAL(op)
	if err != nil {
		return nil, err
	}
	table := &Table{
		Name:    name,
		Columns: columns,
		Rows:    make(map[string]Row),
	}
	db.tables[name] = table
	db.lastOpNumber = op.OpNumber
	if db.lastOpNumber%10 == 0 {
		select {
		case db.snapshotChan <- struct{}{}:
		default:
		}
	}
	return table, nil
}

func (db *Database) insertRow(tableName string, rowID string, row Row) (Row, error) {
	db.mu.RLock()
	table, exists := db.tables[tableName]
	db.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("table does not exist")
	}
	table.lock.Lock()
	defer table.lock.Unlock()
	_, exists = table.Rows[rowID]
	if exists {
		return nil, fmt.Errorf("row already exists")
	}
	for _, column := range table.Columns {
		value, exists := row[column.Name]
		if !exists {
			return nil, fmt.Errorf("missing column: %s", column.Name)
		}
		switch column.Type {

		case "string":
			_, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("column %s must be string", column.Name)
			}

		case "int":
			number, ok := value.(float64)
			if !ok {
				return nil, fmt.Errorf("column %s must be int", column.Name)
			}

			if number != float64(int(number)) {
				return nil, fmt.Errorf("column %s must be integer", column.Name)
			}

		case "bool":
			_, ok := value.(bool)
			if !ok {
				return nil, fmt.Errorf("column %s must be bool", column.Name)
			}
		}
	}

	for key := range row {
		found := false
		for _, column := range table.Columns {
			if column.Name == key {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("unknown column: %s", key)
		}
	}
	op := WAL{
		OpNumber:  db.lastOpNumber + 1,
		Operation: "INSERT_ROW",
		TableName: tableName,
		RowID:     rowID,
		RowData:   row,
		Timestamp: time.Now(),
	}
	err := db.appendWAL(op)
	if err != nil {
		return nil, err
	}
	table.Rows[rowID] = row
	db.lastOpNumber = op.OpNumber
	if db.lastOpNumber%10 == 0 {
		select {
		case db.snapshotChan <- struct{}{}:
		default:
		}
	}
	return row, nil
}

func (db *Database) deleteTable(name string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	_, exists := db.tables[name]
	if !exists {
		return fmt.Errorf("table does not exist")
	}
	op := WAL{
		OpNumber:  db.lastOpNumber + 1,
		Operation: "DELETE_TABLE",
		TableName: name,
		Timestamp: time.Now(),
	}
	err := db.appendWAL(op)
	if err != nil {
		return err
	}
	delete(db.tables, name)
	db.lastOpNumber = op.OpNumber
	return nil
}

func (db *Database) deleteRow(tableName, rowId string) error {
	db.mu.Lock()
	table, exists := db.tables[tableName]
	db.mu.Unlock()
	if !exists {
		return fmt.Errorf("table does not exist")
	}
	table.lock.Lock()
	defer table.lock.Unlock()
	_, exists = table.Rows[rowId]
	if !exists {
		return fmt.Errorf("row does not exist")
	}
	op := WAL{
		OpNumber:  db.lastOpNumber + 1,
		Operation: "DELETE_ROW",
		TableName: tableName,
		RowID:     rowId,
		Timestamp: time.Now(),
	}
	err := db.appendWAL(op)
	if err != nil {
		return err
	}
	delete(table.Rows, rowId)
	db.lastOpNumber = op.OpNumber
	return nil
}

func (db *Database) getTables() []*Table {
	db.mu.RLock()
	defer db.mu.RUnlock()
	var result []*Table
	for _, table := range db.tables {
		result = append(result, table)
	}
	return result
}

func (db *Database) getTable(tableName string) (*Table, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	table, ok := db.tables[tableName]
	if !ok {
		return nil, fmt.Errorf("table does not exist")
	}
	return table, nil
}

func (db *Database) getRow(tableName, rowId string) (Row, error) {
	db.mu.RLock()
	table, ok := db.tables[tableName]
	db.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("table does not exist")
	}
	table.lock.RLock()
	defer table.lock.RUnlock()
	row, ok := table.Rows[rowId]
	if !ok {
		return nil, fmt.Errorf("row does not exist")
	}
	return row, nil
}

func (db *Database) appendWAL(op WAL) error {
	bytes, err := json.Marshal(op)
	if err != nil {
		return err
	}
	_, err = db.walFile.Write(append(bytes, '\n'))
	if err != nil {
		return err
	}
	return db.walFile.Sync()
}

func (db *Database) createSnapshot() error {
	db.mu.RLock()
	defer db.mu.RUnlock()
	file, err := os.Create("snapshot.json")
	if err != nil {
		return err
	}
	defer file.Close()
	snap := Snapshot{
		LastOpNumber: db.lastOpNumber,
		Items:        db.tables,
	}
	err = json.NewEncoder(file).Encode(snap)
	if err != nil {
		return err
	}
	err = file.Sync()
	if err != nil {
		return err
	}
	db.walFile.Close()
	wal, err := os.OpenFile(
		"wal.log",
		os.O_CREATE|os.O_RDWR|os.O_TRUNC,
		0644,
	)
	if err != nil {
		return err
	}
	db.walFile = wal
	return nil
}

func (db *Database) loadSnapshot() error {
	_, err := os.Stat("snapshot.json")
	if os.IsNotExist(err) {
		return nil
	}
	file, err := os.Open("snapshot.json")
	if err != nil {
		return err
	}
	defer file.Close()
	var snap Snapshot
	err = json.NewDecoder(file).Decode(&snap)
	if err != nil {
		return err
	}
	if snap.Items == nil {
		db.tables = make(map[string]*Table)
	} else {
		db.tables = snap.Items
	}

	db.lastOpNumber = snap.LastOpNumber

	return nil
}

func (db *Database) loadWAL() error {
	_, err := db.walFile.Seek(0, 0)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(db.walFile)
	for {
		var op WAL
		err := decoder.Decode(&op)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if op.OpNumber <= db.lastOpNumber {
			continue
		}
		switch op.Operation {

		case "CREATE_TABLE":
			db.tables[op.TableName] = &Table{
				Name:    op.TableName,
				Columns: op.Columns,
				Rows:    make(map[string]Row),
			}

		case "INSERT_ROW":
			table, exists := db.tables[op.TableName]
			if exists {
				table.Rows[op.RowID] = op.RowData
			}

		case "DELETE_ROW":
			table, exists := db.tables[op.TableName]
			if exists {
				delete(table.Rows, op.RowID)
			}

		case "DELETE_TABLE":
			delete(db.tables, op.TableName)
		}

		db.lastOpNumber = op.OpNumber
	}
	return nil
}

func (db *Database) snapshotWorker() {
	for range db.snapshotChan {
		err := db.createSnapshot()

		if err != nil {
			fmt.Println(err)
		}
	}
}

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
	r.Post("/tables/{name}", createTable)
	r.Post("/tables/{tableName}/rows/{rowId}", insertRow)
	r.Delete("/tables/{name}", deleteTable)
	r.Delete("/tables/{tableName}/row/{rowId}", deleteRow)
	r.Get("/tables", getTables)
	r.Get("/tables/{tableName}", getTable)
	r.Get("/tables/{tableName}/rows/{rowId}", getRow)
	fmt.Println("Server running on :8080")
	err = http.ListenAndServe(":8080", r)
	if err != nil {
		panic(err)
	}
}

func createTable(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	var req CreateTableRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "invalid json", 400)
		return
	}
	table, err := db.createTable(name, req.Columns)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(table)
}

func insertRow(w http.ResponseWriter, r *http.Request) {
	tableName := chi.URLParam(r, "tableName")
	rowID := chi.URLParam(r, "rowId")

	var row Row

	err := json.NewDecoder(r.Body).Decode(&row)
	if err != nil {
		http.Error(w, "invalid json", 400)
		return
	}

	insertedRow, err := db.insertRow(tableName, rowID, row)

	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(insertedRow)
}

func deleteTable(w http.ResponseWriter, r *http.Request) {
	tableName := chi.URLParam(r, "name")
	err := db.deleteTable(tableName)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Write([]byte("deleted table"))
}

func deleteRow(w http.ResponseWriter, r *http.Request) {
	tableName := chi.URLParam(r, "tableName")
	rowId := chi.URLParam(r, "rowId")
	err := db.deleteRow(tableName, rowId)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Write([]byte("deleted row"))
}

func getTables(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var result []*Table
	result = db.getTables()
	json.NewEncoder(w).Encode(result)
}

func getTable(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	tableName := chi.URLParam(r, "tableName")
	table, err := db.getTable(tableName)
	if err != nil {
		http.Error(w, err.Error(), 404)
	}
	json.NewEncoder(w).Encode(table)
}

func getRow(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	tableName := chi.URLParam(r, "tableName")
	rowId := chi.URLParam(r, "rowId")
	row, err := db.getRow(tableName, rowId)
	if err != nil {
		http.Error(w, err.Error(), 404)
	}
	json.NewEncoder(w).Encode(row)
}
