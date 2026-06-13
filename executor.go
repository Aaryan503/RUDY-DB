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

func evalNode(node *ExprNode, row Row) (bool, error) {
	if node == nil {
		return true, nil
	}
	if node.Condition != nil {
		val, ok := row[node.Condition.Field]
		if !ok {
			return false, nil
		}
		return evaluateThis(val, node.Condition.Operator, node.Condition.Value)
	}
	left, err := evalNode(node.Left, row)
	if err != nil {
		return false, err
	}
	if node.Op == "OR" && left {
		return true, nil
	}
	if node.Op == "AND" && !left {
		return false, nil
	}
	if node.Op == "NOT" {
		return !left, nil
	}
	return evalNode(node.Right, row)
}

func filter(where *WhereClause, columns []Column) (func(Row) bool, error) {
	if where == nil {
		return nil, nil
	}
	err := validateNode(where.root, columns)
	if err != nil {
		return nil, err
	}
	return func(row Row) bool {
		result, _ := evalNode(where.root, row)
		return result
	}, nil
}

func validateNode(node *ExprNode, columns []Column) error {
	if node == nil {
		return nil
	}
	if node.Condition != nil {
		for _, col := range columns {
			if col.Name == node.Condition.Field {
				return nil
			}
		}
		return fmt.Errorf("unknown column in WHERE: %s", node.Condition.Field)
	}
	err := validateNode(node.Left, columns)
	if err != nil {
		return err
	}
	return validateNode(node.Right, columns)
}

func evaluateThis(rowValue interface{}, op, field string) (bool, error) {
	switch v := rowValue.(type) {
	case string:
		switch op {
		case "=":
			return (v == field), nil
		case "!=":
			return (v != field), nil
		case ">":
			return (v > field), nil
		case "<":
			return (v < field), nil
		case ">=":
			return (v >= field), nil
		case "<=":
			return (v <= field), nil
		default:
			return false, fmt.Errorf("operator %s not supported for string", op)
		}
	case float64:
		n, err := strconv.ParseFloat(field, 64)
		if err != nil {
			return false, fmt.Errorf("invalid number in WHERE: %s", field)
		}
		switch op {
		case "=":
			return v == n, nil
		case "!=":
			return v != n, nil
		case "<":
			return v < n, nil
		case ">":
			return v > n, nil
		case "<=":
			return v <= n, nil
		case ">=":
			return v >= n, nil
		}
	case bool:
		b, err := strconv.ParseBool(field)
		if err != nil {
			return false, fmt.Errorf("invalid bool in where condition: %s", field)
		}
		switch op {
		case "=":
			return v == b, nil
		case "!=":
			return v != b, nil
		default:
			return false, fmt.Errorf("operator %s not supported for bool", op)
		}
	}
	return false, fmt.Errorf("unsupported type in where")

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
		return e.db.insertRow(s.TableName, newRow)

	case *UpdateStatement:
		e.db.mu.RLock()
		table, exists := e.db.tables[s.TableName]
		e.db.mu.RUnlock()
		if !exists {
			return nil, fmt.Errorf("table does not exist")
		}

		filterFn, err := filter(s.Where, table.Columns)
		if err != nil {
			return nil, err
		}
		table.lock.RLock()
		var toUpdate []string
		for id, row := range table.Rows {
			if filterFn == nil || filterFn(row) {
				toUpdate = append(toUpdate, id)
			}
		}
		table.lock.RUnlock()

		count := 0
		for _, id := range toUpdate {
			_, err := e.db.updateRow(s.TableName, id, s.Updates)
			if err != nil {
				return nil, err
			}
			count++
		}
		return map[string]int{"updated": count}, nil

	case *DeleteStatement:
		e.db.mu.RLock()
		table, ok := e.db.tables[s.TableName]
		e.db.mu.RUnlock()
		if !ok {
			return nil, fmt.Errorf("Table does not exist")
		}
		filter, err := filter(s.Where, table.Columns)
		if err != nil {
			return nil, err
		}
		table.lock.RLock()
		var toDelete []string
		for id, row := range table.Rows {
			if filter == nil || filter(row) {
				toDelete = append(toDelete, id)
			}
		}
		table.lock.RUnlock()
		for _, id := range toDelete {
			if err := e.db.deleteRow(s.TableName, id); err != nil {
				return nil, err
			}
		}
		return map[string]int{"deleted": len(toDelete)}, nil

	case *SelectStatement:
		e.db.mu.RLock()
		table, exists := e.db.tables[s.TableName]
		e.db.mu.RUnlock()
		if !exists {
			return nil, fmt.Errorf("table does not exist")
		}
		if s.Star {
			for _, col := range table.Columns {
				s.Fields = append(s.Fields, col.Name)
			}
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
		filter, err := filter(s.Where, table.Columns)
		if err != nil {
			return nil, err
		}
		return e.db.selectRows(s.TableName, s.Fields, filter, s.Limit, s.Distinct)

	case *DropStatement:
		e.db.mu.RLock()
		table, exists := e.db.tables[s.TableName]
		e.db.mu.RUnlock()
		if !exists {
			return nil, fmt.Errorf("table does not exist")
		}
		err := e.db.deleteTable(s.TableName)
		return map[string]int{"deleted": len(table.Rows)}, err

	default:
		return nil, fmt.Errorf("unknown statement type")
	}
}
