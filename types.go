package main

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type ValueType string

const (
	ValueTypeString ValueType = "string"
	ValueTypeInt    ValueType = "int"
	ValueTypeBool   ValueType = "bool"
)

type Value struct {
	Type ValueType
	Data interface{}
}

func (v Value) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.Data)
}

type Row map[string]Value

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

type WAL struct {
	OpNumber  int              `json:"opNumber"`
	Operation string           `json:"operation"`
	TableName string           `json:"tableName"`
	RowID     string           `json:"rowId,omitempty"`
	RowData   map[string]Value `json:"rowData,omitempty"`
	Columns   []Column         `json:"columns,omitempty"`
	Timestamp time.Time        `json:"timestamp"`
}

type Snapshot struct {
	LastOpNumber int               `json:"lastOpNumber"`
	Items        map[string]*Table `json:"items"`
}

type CreateTableRequest struct {
	Columns []Column `json:"columns"`
}

func toValue(colName, colType string, raw interface{}) (Value, error) {
	switch colType {
	case "string":
		s, ok := raw.(string)
		if !ok {
			return Value{}, fmt.Errorf("column %s must be string", colName)
		}
		return Value{Type: ValueTypeString, Data: s}, nil

	case "int":
		f, ok := raw.(float64)
		if !ok {
			return Value{}, fmt.Errorf("column %s must be int", colName)
		}
		if f != float64(int(f)) {
			return Value{}, fmt.Errorf("column %s must be integer", colName)
		}
		return Value{Type: ValueTypeInt, Data: int(f)}, nil

	case "bool":
		b, ok := raw.(bool)
		if !ok {
			return Value{}, fmt.Errorf("column %s must be bool", colName)
		}
		return Value{Type: ValueTypeBool, Data: b}, nil

	default:
		return Value{}, fmt.Errorf("unsupported type: %s", colType)
	}
}
