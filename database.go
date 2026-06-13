package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
	"time"
)

type Database struct {
	mu           sync.RWMutex
	tables       map[string]*Table
	walFile      *os.File
	lastOpNumber int
	snapshotChan chan struct{}
}

func validType(t string) bool {
	switch t {
	case "string", "int", "bool", "float":
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

func (db *Database) insertRow(tableName string, row Row) (Row, error) {
	db.mu.RLock()
	table, exists := db.tables[tableName]
	db.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("table does not exist")
	}
	table.lock.Lock()
	defer table.lock.Unlock()
	table.NextRowId++
	rowID := fmt.Sprintf("row %f", table.NextRowId)
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
		case "float":
			_, ok := value.(float64)
			if !ok {
				return nil, fmt.Errorf("column %s must be a float", column.Name)
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

func (db *Database) updateRow(tableName, rowId string, updates map[string]string) (Row, error) {
	db.mu.RLock()
	table, exists := db.tables[tableName]
	db.mu.RUnlock()
	if !exists {
		return nil, fmt.Errorf("table does not exist")
	}
	table.lock.Lock()
	defer table.lock.Unlock()
	existing, exists := table.Rows[rowId]
	if !exists {
		return nil, fmt.Errorf("row does not exist")
	}
	updatedRow := make(Row)
	for k, v := range existing {
		updatedRow[k] = v
	}
	for colName, rawVal := range updates {
		var col *Column
		for i := range table.Columns {
			if table.Columns[i].Name == colName {
				col = &table.Columns[i]
				break
			}
		}
		if col == nil {
			return nil, fmt.Errorf("unknown column: %s", colName)
		}
		switch col.Type {
		case "string":
			updatedRow[colName] = rawVal
		case "int":
			n, err := strconv.ParseFloat(rawVal, 64)
			if err != nil || n != float64(int(n)) {
				return nil, fmt.Errorf("column %s must be int", colName)
			}
			updatedRow[colName] = n
		case "float":
			n, err := strconv.ParseFloat(rawVal, 64)
			if err != nil {
				return nil, fmt.Errorf("column %s must be float", colName)
			}
			updatedRow[colName] = n
		case "bool":
			b, err := strconv.ParseBool(rawVal)
			if err != nil {
				return nil, fmt.Errorf("column %s must be bool", colName)
			}
			updatedRow[colName] = b
		}
	}

	op := WAL{
		OpNumber:  db.lastOpNumber + 1,
		Operation: "UPDATE_ROW",
		TableName: tableName,
		RowID:     rowId,
		RowData:   updatedRow,
		Timestamp: time.Now(),
	}
	if err := db.appendWAL(op); err != nil {
		return nil, err
	}

	table.Rows[rowId] = updatedRow
	db.lastOpNumber = op.OpNumber
	return updatedRow, nil
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
	db.mu.RLock()
	table, exists := db.tables[tableName]
	db.mu.RUnlock()
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

func (db *Database) selectRows(tableName string, fields []string, filter func(Row) bool, limit int, distinct bool) (*SelectResult, error) {
	table, err := db.getTable(tableName)
	if err != nil {
		return nil, err
	}
	table.lock.RLock()
	defer table.lock.RUnlock()
	var rows []Row
	var seen map[string]struct{}
	if distinct {
		seen = make(map[string]struct{})
	}
	for _, row := range table.Rows {
		if filter != nil && !filter(row) {
			continue
		}
		filteredRow := make(Row, len(fields))

		for _, field := range fields {
			if val, ok := row[field]; ok {
				filteredRow[field] = val
			}
		}
		if distinct {
			key := rowKey(filteredRow, fields)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
		}
		rows = append(rows, filteredRow)
		if limit > 0 && len(rows) >= limit {
			break
		}
	}
	return &SelectResult{
		Columns: fields,
		Rows:    rows,
	}, nil
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
	file, err := os.Create("snapshot.json")
	if err != nil {
		return err
	}
	defer file.Close()
	snap := Snapshot{
		LastOpNumber: db.lastOpNumber,
		Items:        db.tables,
	}
	db.mu.RUnlock()
	err = json.NewEncoder(file).Encode(snap)
	if err != nil {
		return err
	}
	err = file.Sync()
	if err != nil {
		return err
	}
	db.mu.Lock()
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
	db.mu.Unlock()
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
		case "UPDATE_ROW":
			table, exists := db.tables[op.TableName]
			if exists {
				table.Rows[op.RowID] = op.RowData
			}
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

func rowKey(row Row, fields []string) string {
	values := make([]interface{}, 0, len(fields))

	for _, field := range fields {
		values = append(values, row[field])
	}

	b, _ := json.Marshal(values)
	return string(b)
}
