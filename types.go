package main

import (
	"sync"
	"time"
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

type CreateTableRequest struct {
	Columns []Column `json:"columns"`
}

type Condition struct {
	Field    string
	Operator string
	Value    string
}
