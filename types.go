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

type Condition struct {
	Field    string
	Operator string
	Value    string
}
