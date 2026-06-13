package main

import (
	"fmt"
	"strconv"
)

type Executor struct {
	db *Database
}

func newExecutor(db *Database) *Executor {
	return &Executor{db: db}
}

func (e *Executor) execute(query string) (interface{}, error) {
	l := newLexer(query)
	p := newParser(l)
	stmt, err := p.parseStatement()
	if err != nil {
		return nil, err
	}
	switch s := stmt.(type) {
	case *CreateTableStatement:
		return e.db.createTable(s.TableName, s.Columns)

	case *InsertStatement:
		e.db.mu.RLock()
		table, exists := e.db.tables[s.TableName]
		e.db.mu.RUnlock()
		if !exists {
			return nil, fmt.Errorf("table does not exist")
		}
		if len(s.Values) != len(table.Columns) {
			return nil, fmt.Errorf("column count mismatch: expected %d, got %d", len(table.Columns), len(s.Values))
		}
		newRow := make(Row)
		for i, column := range table.Columns {
			valStr, ok := s.Values[i].(string)
			if !ok {
				return nil, fmt.Errorf("expected string value from parser")
			}

			switch column.Type {
			case "int":
				number, err := strconv.ParseFloat(valStr, 64)
				if err != nil {
					return nil, fmt.Errorf("column %s must be int, got %s", column.Name, valStr)
				}
				if number != float64(int(number)) {
					return nil, fmt.Errorf("column %s must be integer", column.Name)
				}
				newRow[column.Name] = number
			case "float":
				valFloat, err := strconv.ParseFloat(valStr, 64)
				if err != nil {
					return nil, fmt.Errorf("column %s must be float, got %s", column.Name, valStr)
				}
				newRow[column.Name] = valFloat
			case "bool":
				valBool, err := strconv.ParseBool(valStr)
				if err != nil {
					return nil, fmt.Errorf("column %s must be bool, got %s", column.Name, valStr)
				}
				newRow[column.Name] = valBool

			default:
				newRow[column.Name] = valStr
			}
		}
		table.lock.RLock()
		rowId := fmt.Sprintf("row%d", len(table.Rows)+1)
		table.lock.RUnlock()
		return e.db.insertRow(s.TableName, rowId, newRow)

	case *SelectStatement:
		e.db.mu.RLock()
		table, exists := e.db.tables[s.TableName]
		e.db.mu.RUnlock()
		if !exists {
			return nil, fmt.Errorf("table does not exist")
		}
		for _, field := range s.Fields {
			fieldExists := false
			for _, col := range table.Columns {
				if col.Name == field {
					fieldExists = true
					break
				}
			}
			if !fieldExists {
				return nil, fmt.Errorf("unknown column: %s", field)
			}
		}
		var whereFilter func(Row) bool
		return e.db.selectRows(s.TableName, s.Fields, whereFilter)

	default:
		return nil, fmt.Errorf("unknown statement type")
	}
}
