# RUDY DB

RUDY is a simple database engine built from scratch in Go. It supports a subset of SQL through two interfaces — a REST API and an interactive CLI — and is designed around three core properties:

- **Persistent** — All changes survive process restarts. Writes go to a Write-Ahead Log (WAL) before being applied, and periodic snapshots compact the log. On startup, RUDY replays any WAL entries after the last snapshot to restore exact state.
- **Concurrent** — RUDY uses two-level locking: a database-level lock for table map access, and a per-table lock for row access. Reads on different tables never block each other.
- **Recoverable** — If the process crashes mid-operation, the WAL ensures no committed write is lost. The snapshot + WAL replay model guarantees consistent recovery.

---

## Table of Contents

- [Setup](#setup)
- [Supported SQL](#supported-sql)
- [API Mode](#api-mode)
  - [REST Routes](#rest-routes)
  - [SQL Query Endpoint](#sql-query-endpoint)
- [CLI Mode](#cli-mode)
- [Caveats & Limitations](#caveats--limitations)

---

## Setup

Ensure Go is installed from [go.dev](https://go.dev/dl/), then:

```bash
git clone https://github.com/Aaryan503/RUDY-DB
cd RUDY-DB
go mod tidy
go run .
```

The server starts at `http://localhost:8080` by default. The CLI launches in the same terminal session.

---

## Supported SQL

| Statement | Syntax |
|---|---|
| Create table | `CREATE TABLE name (col1 type, col2 type, ...)` |
| Insert row | `INSERT INTO name VALUES (val1, val2, ...)` |
| Select all | `SELECT * FROM name` |
| Select columns | `SELECT col1, col2 FROM name` |
| Select with filter | `SELECT * FROM name WHERE col op val` |
| Select with limit | `SELECT col1,col2 FROM name WHERE col1 op val LIMIT number` |
| Delete with filter | `DELETE FROM name WHERE col op val` |
| Delete all rows | `DELETE FROM name` |
| Update rows | `UPDATE name SET col = val WHERE col op val` |
| Update multiple columns | `UPDATE name SET col1 = val1, col2 = val2 WHERE col op val` |
| Update all rows | `UPDATE name SET col = val` |
| Drop table | `DROP name` |

**Supported column types:** `string`, `int`, `float`, `bool`, along with negative numbers

**Supported WHERE operators:** `=`, `!=`, `<`, `>`, `<=`, `>=`

Multiple WHERE conditions can be combined with `AND`, and result can be limited with `LIMIT`

>[!WARNING]
>`OR` in WHERE statements, and aggregate operators are not supported yet
---

## API Mode

The server exposes two styles of endpoint: structured REST routes for direct row/table operations, and a single SQL query endpoint that accepts raw SQL strings.

### REST Routes

#### Tables

**Create a table**
```
POST /tables/{name}
```
```bash
curl -X POST http://localhost:8080/tables/users \
  -H "Content-Type: application/json" \
  -d "{\"columns\": [{\"name\": \"name\", \"type\": \"string\"}, {\"name\": \"age\", \"type\": \"int\"}]}"
```

**Get all tables**
```
GET /tables
```
```bash
curl http://localhost:8080/tables
```

**Get a specific table**
```
GET /tables/{tableName}
```
```bash
curl http://localhost:8080/tables/users
```

**Drop a table**
```
DELETE /tables/{name}
```
```bash
curl -X DELETE http://localhost:8080/tables/users
```

---

#### Rows

**Insert a row**
```
POST /tables/{tableName}/rows/{rowId}
```
```bash
curl -X POST http://localhost:8080/tables/users/rows/row1 \
  -H "Content-Type: application/json" \
  -d "{\"name\": \"alice\", \"age\": 30}"
```

**Get a row**
```
GET /tables/{tableName}/rows/{rowId}
```
```bash
curl http://localhost:8080/tables/users/rows/row1
```

**Update a row**
```
PUT /tables/{tableName}/rows/{rowId}
```
```bash
curl -X PUT http://localhost:8080/tables/users/rows/row1 \
  -H "Content-Type: application/json" \
  -d "{\"updates\": {\"age\": \"31\"}}"
```

**Delete a row**
```
DELETE /tables/{tableName}/row/{rowId}
```
```bash
curl -X DELETE http://localhost:8080/tables/users/row/row1
```

---

### SQL Query Endpoint

All supported SQL statements can be sent as plain text to a single endpoint. This is the recommended interface for complex queries.

```
POST /query
```

```bash
# Create table
curl -X POST http://localhost:8080/query -d "CREATE TABLE products (name string, price float, inStock bool)"

# Insert rows
curl -X POST http://localhost:8080/query -d "INSERT INTO products VALUES ('keyboard', 49.99, true)"

# Select
curl -X POST http://localhost:8080/query -d "SELECT name,price FROM products WHERE price > 20 AND inStock = true LIMIT 3"

# Update
curl -X POST http://localhost:8080/query -d "UPDATE products SET price = 279.99, inStock = false WHERE name = 'monitor'"

# Delete
curl -X POST http://localhost:8080/query -d "DELETE FROM products WHERE inStock = false"

# Delete all rows
curl -X POST http://localhost:8080/query -d "DELETE FROM products"

# Drop table
curl -X POST http://localhost:8080/query -d "DROP products"
```

---

## CLI Mode

When you run `go run .`, RUDY starts an interactive SQL prompt in the terminal alongside the API server. Type any supported SQL statement and press Enter. Type `Ctrl+C` to quit.

```
```text
rudydb> CREATE TABLE employees (name string, department string, salary float, active bool)
Table: employees (4 columns)

rudydb> INSERT INTO employees VALUES ('alice', 'engineering', 95000.00, true)
Inserted:
  name = alice
  department = engineering
  salary = 95000
  active = true

rudydb> INSERT INTO employees VALUES ('bob', 'marketing', 72000.00, true)
Inserted:
  name = bob
  department = marketing
  salary = 72000
  active = true

rudydb> INSERT INTO employees VALUES ('carol', 'engineering', 110000.00, true)
Inserted:
  name = carol
  department = engineering
  salary = 110000
  active = true

rudydb> INSERT INTO employees VALUES ('dave', 'sales', 65000.00, false)
Inserted:
  name = dave
  department = sales
  salary = 65000
  active = false

rudydb> SELECT * FROM employees WHERE salary > 80000 AND active = true

name           department     salary         active
-------------- -------------- -------------- --------------
alice          engineering    95000          true
carol          engineering    110000         true

2 row(s)

rudydb> UPDATE employees SET salary = 100000.00 WHERE name = 'bob'
updated: 1

rudydb> SELECT name,salary FROM employees LIMIT 3

name           salary
-------------- --------------
alice          95000
bob            100000
carol          110000

3 row(s)

rudydb> DELETE FROM employees WHERE active = false
deleted: 1

rudydb> DROP employees
Done
```


## Caveats & Limitations

**Query support**
- WHERE clauses support simple comparisons only. Conditions are `AND`ed together; `OR` is not yet supported, however it will be the next major feature to be added.
- No `JOIN`, `GROUP BY`, `ORDER BY`, `LIMIT`, or aggregate functions (`COUNT`, `SUM`, etc.) are implemented yet, will be supported soon.

**Types**
- `int` and `float` are both stored as `float64` internally (Go's JSON representation). Very large integers may lose precision.
- There is no `NULL` support. yet

**Concurrency**
- Concurrent reads on the same table are safe. Concurrent writes serialize at the table lock. Concurrent writes across different tables are fully parallel.
