package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

//NOTE: THESE TESTS ARE AI GENERATED AND MEANT TO SERVE AS A QUICK SANITY CHECK FOR
//OR WHOEVER IS RUNNING IT, TO VERIFY EVERYTHING IS WORKING AND NOTHING IS BROKEN

// --------------------------------------------------------------------------
// helpers
// --------------------------------------------------------------------------

func newTestDB(t *testing.T) *Database {
	t.Helper()
	dir := t.TempDir()

	walPath := filepath.Join(dir, "wal.log")
	snapPath := filepath.Join(dir, "snapshot.json")

	wal, err := os.OpenFile(walPath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}

	db := &Database{
		tables:       make(map[string]*Table),
		walFile:      wal,
		snapshotChan: make(chan struct{}, 1),
	}

	t.Cleanup(func() {
		wal.Close()
		os.Remove(walPath)
		os.Remove(snapPath)
	})

	return db
}

func newTestExecutor(t *testing.T) (*Executor, *Database) {
	t.Helper()
	db := newTestDB(t)
	return newExecutor(db), db
}

// mustExec runs a query and fails the test on error.
func mustExec(t *testing.T, e *Executor, query string) interface{} {
	t.Helper()
	res, err := e.execute(query)
	if err != nil {
		t.Fatalf("execute(%q): %v", query, err)
	}
	return res
}

// mustFail asserts a query returns an error.
func mustFail(t *testing.T, e *Executor, query string) {
	t.Helper()
	_, err := e.execute(query)
	if err == nil {
		t.Fatalf("expected error for query %q, got nil", query)
	}
}

func asSelect(t *testing.T, v interface{}) *SelectResult {
	t.Helper()
	r, ok := v.(*SelectResult)
	if !ok {
		t.Fatalf("expected *SelectResult, got %T", v)
	}
	return r
}

func asRow(t *testing.T, v interface{}) Row {
	t.Helper()
	r, ok := v.(Row)
	if !ok {
		t.Fatalf("expected Row, got %T", v)
	}
	return r
}

func setup(t *testing.T) *Executor {
	t.Helper()
	e, _ := newTestExecutor(t)
	mustExec(t, e, `CREATE TABLE users (id int, name string, age int, active bool)`)
	mustExec(t, e, `INSERT INTO users VALUES (1, Alice, 30, true)`)
	mustExec(t, e, `INSERT INTO users VALUES (2, Bob, 25, false)`)
	mustExec(t, e, `INSERT INTO users VALUES (3, Charlie, 30, true)`)
	mustExec(t, e, `INSERT INTO users VALUES (4, Diana, 22, false)`)
	mustExec(t, e, `INSERT INTO users VALUES (5, Eve, 25, true)`)
	return e
}

// --------------------------------------------------------------------------
// CREATE TABLE
// --------------------------------------------------------------------------

func TestCreateTable(t *testing.T) {
	e, _ := newTestExecutor(t)

	mustExec(t, e, `CREATE TABLE people (id int, name string)`)

	// duplicate table
	mustFail(t, e, `CREATE TABLE people (id int)`)
}

func TestCreateTable_InvalidType(t *testing.T) {
	e, _ := newTestExecutor(t)
	mustFail(t, e, `CREATE TABLE bad (id uuid)`)
}

func TestCreateTable_DuplicateColumn(t *testing.T) {
	e, _ := newTestExecutor(t)
	mustFail(t, e, `CREATE TABLE bad (id int, id string)`)
}

// --------------------------------------------------------------------------
// INSERT
// --------------------------------------------------------------------------

func TestInsert_Basic(t *testing.T) {
	e, _ := newTestExecutor(t)
	mustExec(t, e, `CREATE TABLE t (id int, name string)`)
	res := mustExec(t, e, `INSERT INTO t VALUES (1, Alice)`)
	row := asRow(t, res)
	if row["name"] != "Alice" {
		t.Fatalf("expected Alice, got %v", row["name"])
	}
}

func TestInsert_ColumnCountMismatch(t *testing.T) {
	e, _ := newTestExecutor(t)
	mustExec(t, e, `CREATE TABLE t (id int, name string)`)
	mustFail(t, e, `INSERT INTO t VALUES (1)`)
}

func TestInsert_WrongType(t *testing.T) {
	e, _ := newTestExecutor(t)
	mustExec(t, e, `CREATE TABLE t (id int, score float)`)
	mustFail(t, e, `INSERT INTO t VALUES (1, notafloat)`)
}

func TestInsert_BoolColumn(t *testing.T) {
	e, _ := newTestExecutor(t)
	mustExec(t, e, `CREATE TABLE t (id int, active bool)`)
	mustExec(t, e, `INSERT INTO t VALUES (1, true)`)
	mustExec(t, e, `INSERT INTO t VALUES (2, false)`)
	mustFail(t, e, `INSERT INTO t VALUES (3, maybe)`)
}

func TestInsert_FloatColumn(t *testing.T) {
	e, _ := newTestExecutor(t)
	mustExec(t, e, `CREATE TABLE t (id int, score float)`)
	mustExec(t, e, `INSERT INTO t VALUES (1, 3.14)`)
}

func TestInsert_IntoNonExistentTable(t *testing.T) {
	e, _ := newTestExecutor(t)
	mustFail(t, e, `INSERT INTO ghost VALUES (1, foo)`)
}

// --------------------------------------------------------------------------
// SELECT basic
// --------------------------------------------------------------------------

func TestSelect_Star(t *testing.T) {
	e := setup(t)
	res := asSelect(t, mustExec(t, e, `SELECT * FROM users`))
	if len(res.Rows) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(res.Rows))
	}
}

func TestSelect_Fields(t *testing.T) {
	e := setup(t)
	res := asSelect(t, mustExec(t, e, `SELECT name, age FROM users`))
	if len(res.Columns) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(res.Columns))
	}
}

func TestSelect_UnknownColumn(t *testing.T) {
	e := setup(t)
	mustFail(t, e, `SELECT ghost FROM users`)
}

func TestSelect_FromNonExistentTable(t *testing.T) {
	e, _ := newTestExecutor(t)
	mustFail(t, e, `SELECT * FROM ghost`)
}

// --------------------------------------------------------------------------
// SELECT LIMIT
// --------------------------------------------------------------------------

func TestSelect_Limit(t *testing.T) {
	e := setup(t)
	res := asSelect(t, mustExec(t, e, `SELECT * FROM users LIMIT 3`))
	if len(res.Rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(res.Rows))
	}
}

func TestSelect_LimitZero(t *testing.T) {
	// LIMIT 0 means no limit in the implementation (limit > 0 check)
	e := setup(t)
	res := asSelect(t, mustExec(t, e, `SELECT * FROM users LIMIT 0`))
	if len(res.Rows) != 5 {
		t.Fatalf("expected 5 rows with LIMIT 0, got %d", len(res.Rows))
	}
}

func TestSelect_LimitBeyondCount(t *testing.T) {
	e := setup(t)
	res := asSelect(t, mustExec(t, e, `SELECT * FROM users LIMIT 100`))
	if len(res.Rows) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(res.Rows))
	}
}

// --------------------------------------------------------------------------
// SELECT DISTINCT
// --------------------------------------------------------------------------

func TestSelect_Distinct(t *testing.T) {
	e := setup(t) // ages: 30,25,30,22,25
	res := asSelect(t, mustExec(t, e, `SELECT DISTINCT age FROM users`))
	if len(res.Rows) != 3 { // 22, 25, 30
		t.Fatalf("expected 3 distinct ages, got %d", len(res.Rows))
	}
}

func TestSelect_DistinctAllUnique(t *testing.T) {
	e := setup(t)
	res := asSelect(t, mustExec(t, e, `SELECT DISTINCT name FROM users`))
	if len(res.Rows) != 5 {
		t.Fatalf("expected 5 distinct names, got %d", len(res.Rows))
	}
}

// --------------------------------------------------------------------------
// WHERE — simple conditions
// --------------------------------------------------------------------------

func TestWhere_Equal_String(t *testing.T) {
	e := setup(t)
	res := asSelect(t, mustExec(t, e, `SELECT * FROM users WHERE name = Alice`))
	if len(res.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(res.Rows))
	}
}

func TestWhere_Equal_Int(t *testing.T) {
	e := setup(t)
	res := asSelect(t, mustExec(t, e, `SELECT * FROM users WHERE age = 25`))
	if len(res.Rows) != 2 { // Bob, Eve
		t.Fatalf("expected 2 rows, got %d", len(res.Rows))
	}
}

func TestWhere_NotEqual(t *testing.T) {
	e := setup(t)
	res := asSelect(t, mustExec(t, e, `SELECT * FROM users WHERE age != 30`))
	if len(res.Rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(res.Rows))
	}
}

func TestWhere_GreaterThan(t *testing.T) {
	e := setup(t)
	res := asSelect(t, mustExec(t, e, `SELECT * FROM users WHERE age > 25`))
	if len(res.Rows) != 2 { // Alice, Charlie (age 30)
		t.Fatalf("expected 2 rows, got %d", len(res.Rows))
	}
}

func TestWhere_LessThan(t *testing.T) {
	e := setup(t)
	res := asSelect(t, mustExec(t, e, `SELECT * FROM users WHERE age < 25`))
	if len(res.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(res.Rows))
	}
}

func TestWhere_GreaterThanOrEqual(t *testing.T) {
	e := setup(t)
	res := asSelect(t, mustExec(t, e, `SELECT * FROM users WHERE age >= 30`))
	if len(res.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(res.Rows))
	}
}

func TestWhere_LessThanOrEqual(t *testing.T) {
	e := setup(t)
	res := asSelect(t, mustExec(t, e, `SELECT * FROM users WHERE age <= 25`))
	if len(res.Rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(res.Rows))
	}
}

func TestWhere_Bool(t *testing.T) {
	e := setup(t)
	res := asSelect(t, mustExec(t, e, `SELECT * FROM users WHERE active = true`))
	if len(res.Rows) != 3 {
		t.Fatalf("expected 3 active users, got %d", len(res.Rows))
	}
}

func TestWhere_UnknownColumn(t *testing.T) {
	e := setup(t)
	mustFail(t, e, `SELECT * FROM users WHERE ghost = 1`)
}

// --------------------------------------------------------------------------
// WHERE — AND / OR / NOT / brackets
// --------------------------------------------------------------------------

func TestWhere_AND(t *testing.T) {
	e := setup(t)
	// age=25 AND active=false → Bob only
	res := asSelect(t, mustExec(t, e, `SELECT * FROM users WHERE age = 25 AND active = false`))
	if len(res.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(res.Rows))
	}
}

func TestWhere_OR(t *testing.T) {
	e := setup(t)
	// age=22 OR age=30 → Diana, Alice, Charlie
	res := asSelect(t, mustExec(t, e, `SELECT * FROM users WHERE age = 22 OR age = 30`))
	if len(res.Rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(res.Rows))
	}
}

func TestWhere_NOT(t *testing.T) {
	e := setup(t)
	// NOT active=true → Bob, Diana
	res := asSelect(t, mustExec(t, e, `SELECT * FROM users WHERE NOT active = true`))
	if len(res.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(res.Rows))
	}
}

func TestWhere_Brackets_ORwithAND(t *testing.T) {
	e := setup(t)
	// (age=25 OR age=22) AND active=false → Bob(25,false) Diana(22,false)
	res := asSelect(t, mustExec(t, e, `SELECT * FROM users WHERE (age = 25 OR age = 22) AND active = false`))
	if len(res.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(res.Rows))
	}
}

func TestWhere_Brackets_Precedence(t *testing.T) {
	e := setup(t)
	// age=30 AND (active=false OR name=Eve)
	// Alice(30,true), Charlie(30,true), Eve(25,true)
	// age=30 AND (false OR name=Eve): neither Alice nor Charlie is Eve → 0
	res := asSelect(t, mustExec(t, e, `SELECT * FROM users WHERE age = 30 AND (active = false OR name = Eve)`))
	if len(res.Rows) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(res.Rows))
	}
}

func TestWhere_AND_OR_Combined(t *testing.T) {
	e := setup(t)
	// name=Alice OR name=Bob AND age=30
	// AND binds tighter: name=Alice OR (name=Bob AND age=30)
	// Bob's age is 25 not 30 → only Alice
	res := asSelect(t, mustExec(t, e, `SELECT * FROM users WHERE name = Alice OR name = Bob AND age = 30`))
	if len(res.Rows) != 1 {
		t.Fatalf("expected 1 row (Alice), got %d", len(res.Rows))
	}
}

// --------------------------------------------------------------------------
// UPDATE
// --------------------------------------------------------------------------

func TestUpdate_Basic(t *testing.T) {
	e := setup(t)
	res := mustExec(t, e, `UPDATE users SET age = 99 WHERE name = Alice`)
	counts := res.(map[string]int)
	if counts["updated"] != 1 {
		t.Fatalf("expected 1 updated, got %d", counts["updated"])
	}
	// verify
	rows := asSelect(t, mustExec(t, e, `SELECT age FROM users WHERE name = Alice`))
	if len(rows.Rows) != 1 {
		t.Fatalf("expected 1 row after update")
	}
	if rows.Rows[0]["age"] != float64(99) {
		t.Fatalf("expected age=99, got %v", rows.Rows[0]["age"])
	}
}

func TestUpdate_MultipleRows(t *testing.T) {
	e := setup(t)
	res := mustExec(t, e, `UPDATE users SET active = true WHERE age = 25`)
	counts := res.(map[string]int)
	if counts["updated"] != 2 { // Bob and Eve
		t.Fatalf("expected 2 updated, got %d", counts["updated"])
	}
}

func TestUpdate_NoWhereUpdatesAll(t *testing.T) {
	e := setup(t)
	res := mustExec(t, e, `UPDATE users SET active = false`)
	counts := res.(map[string]int)
	if counts["updated"] != 5 {
		t.Fatalf("expected 5 updated, got %d", counts["updated"])
	}
}

func TestUpdate_UnknownColumn(t *testing.T) {
	e := setup(t)
	mustFail(t, e, `UPDATE users SET ghost = 1 WHERE name = Alice`)
}

func TestUpdate_NonExistentTable(t *testing.T) {
	e, _ := newTestExecutor(t)
	mustFail(t, e, `UPDATE ghost SET x = 1`)
}

// --------------------------------------------------------------------------
// DELETE rows
// --------------------------------------------------------------------------

func TestDelete_WithWhere(t *testing.T) {
	e := setup(t)
	res := mustExec(t, e, `DELETE FROM users WHERE name = Alice`)
	counts := res.(map[string]int)
	if counts["deleted"] != 1 {
		t.Fatalf("expected 1 deleted, got %d", counts["deleted"])
	}
	rows := asSelect(t, mustExec(t, e, `SELECT * FROM users`))
	if len(rows.Rows) != 4 {
		t.Fatalf("expected 4 rows remaining, got %d", len(rows.Rows))
	}
}

func TestDelete_MultipleRows(t *testing.T) {
	e := setup(t)
	res := mustExec(t, e, `DELETE FROM users WHERE age = 25`)
	counts := res.(map[string]int)
	if counts["deleted"] != 2 {
		t.Fatalf("expected 2 deleted, got %d", counts["deleted"])
	}
}

func TestDelete_NoWhere_DeletesAll(t *testing.T) {
	e := setup(t)
	mustExec(t, e, `DELETE FROM users`)
	rows := asSelect(t, mustExec(t, e, `SELECT * FROM users`))
	if len(rows.Rows) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(rows.Rows))
	}
}

func TestDelete_NonExistentTable(t *testing.T) {
	e, _ := newTestExecutor(t)
	mustFail(t, e, `DELETE FROM ghost`)
}

// --------------------------------------------------------------------------
// DROP TABLE
// --------------------------------------------------------------------------

func TestDrop_Basic(t *testing.T) {
	e := setup(t)
	mustExec(t, e, `DROP TABLE users`)
	mustFail(t, e, `SELECT * FROM users`)
}

func TestDrop_NonExistent(t *testing.T) {
	e, _ := newTestExecutor(t)
	mustFail(t, e, `DROP TABLE ghost`)
}

func TestDrop_ThenRecreate(t *testing.T) {
	e := setup(t)
	mustExec(t, e, `DROP TABLE users`)
	mustExec(t, e, `CREATE TABLE users (id int, name string)`)
	mustExec(t, e, `INSERT INTO users VALUES (1, Fresh)`)
	rows := asSelect(t, mustExec(t, e, `SELECT * FROM users`))
	if len(rows.Rows) != 1 {
		t.Fatalf("expected 1 row after recreate, got %d", len(rows.Rows))
	}
}

// --------------------------------------------------------------------------
// Aggregates
// --------------------------------------------------------------------------

func TestAggregate_COUNT_Star(t *testing.T) {
	e := setup(t)
	row := asRow(t, mustExec(t, e, `SELECT COUNT(*) FROM users`))
	if row["COUNT(*)"] != 5 {
		t.Fatalf("expected COUNT(*)=5, got %v", row["COUNT(*)"])
	}
}

func TestAggregate_COUNT_WithWhere(t *testing.T) {
	e := setup(t)
	row := asRow(t, mustExec(t, e, `SELECT COUNT(*) FROM users WHERE active = true`))
	if row["COUNT(*)"] != 3 {
		t.Fatalf("expected 3, got %v", row["COUNT(*)"])
	}
}

func TestAggregate_SUM(t *testing.T) {
	e := setup(t)
	// ages: 30+25+30+22+25 = 132
	row := asRow(t, mustExec(t, e, `SELECT SUM(age) FROM users`))
	if row["SUM(age)"] != float64(132) {
		t.Fatalf("expected SUM=132, got %v", row["SUM(age)"])
	}
}

func TestAggregate_AVG(t *testing.T) {
	e := setup(t)
	row := asRow(t, mustExec(t, e, `SELECT AVG(age) FROM users`))
	expected := float64(132) / float64(5)
	if row["AVG(age)"] != expected {
		t.Fatalf("expected AVG=%.2f, got %v", expected, row["AVG(age)"])
	}
}

func TestAggregate_MIN(t *testing.T) {
	e := setup(t)
	row := asRow(t, mustExec(t, e, `SELECT MIN(age) FROM users`))
	if row["MIN(age)"] != float64(22) {
		t.Fatalf("expected MIN=22, got %v", row["MIN(age)"])
	}
}

func TestAggregate_MAX(t *testing.T) {
	e := setup(t)
	row := asRow(t, mustExec(t, e, `SELECT MAX(age) FROM users`))
	if row["MAX(age)"] != float64(30) {
		t.Fatalf("expected MAX=30, got %v", row["MAX(age)"])
	}
}

func TestAggregate_Alias(t *testing.T) {
	e := setup(t)
	row := asRow(t, mustExec(t, e, `SELECT COUNT(*) AS total FROM users`))
	if row["total"] != 5 {
		t.Fatalf("expected total=5, got %v", row["total"])
	}
}

func TestAggregate_OnNonNumeric(t *testing.T) {
	e := setup(t)
	mustFail(t, e, `SELECT SUM(name) FROM users`)
}

func TestAggregate_OnEmptyTable(t *testing.T) {
	e, _ := newTestExecutor(t)
	mustExec(t, e, `CREATE TABLE empty (val int)`)
	row := asRow(t, mustExec(t, e, `SELECT COUNT(*) FROM empty`))
	if row["COUNT(*)"] != 0 {
		t.Fatalf("expected 0, got %v", row["COUNT(*)"])
	}
}

// --------------------------------------------------------------------------
// WAL Replay
// --------------------------------------------------------------------------

func TestWALReplay(t *testing.T) {
	dir := t.TempDir()
	walPath := filepath.Join(dir, "wal.log")
	snapPath := filepath.Join(dir, "snapshot.json")

	// --- Phase 1: write data ---
	func() {
		wal, err := os.OpenFile(walPath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
		if err != nil {
			t.Fatalf("open wal: %v", err)
		}
		db := &Database{
			tables:       make(map[string]*Table),
			walFile:      wal,
			snapshotChan: make(chan struct{}, 1),
		}
		e := newExecutor(db)
		mustExec(t, e, `CREATE TABLE things (id int, label string)`)
		mustExec(t, e, `INSERT INTO things VALUES (1, 'alpha')`)
		mustExec(t, e, `INSERT INTO things VALUES (2, 'beta')`)
		mustExec(t, e, `INSERT INTO things VALUES (3, 'gamma')`)
		mustExec(t, e, `UPDATE things SET label = updated WHERE id = 2`)
		mustExec(t, e, `DELETE FROM things WHERE id = 1`)
		wal.Close()
	}()

	// --- Phase 2: fresh DB, replay WAL ---
	func() {
		wal, err := os.OpenFile(walPath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
		if err != nil {
			t.Fatalf("reopen wal: %v", err)
		}
		db := &Database{
			tables:       make(map[string]*Table),
			walFile:      wal,
			snapshotChan: make(chan struct{}, 1),
		}

		_ = snapPath
		if err := db.loadWAL(); err != nil {
			t.Fatalf("loadWAL: %v", err)
		}
		e := newExecutor(db)

		rows := asSelect(t, mustExec(t, e, `SELECT * FROM things`))
		// id=1 deleted, id=2 label=updated, id=3 label=gamma
		if len(rows.Rows) != 2 {
			t.Fatalf("expected 2 rows after WAL replay, got %d", len(rows.Rows))
		}

		// verify label update persisted
		labelRows := asSelect(t, mustExec(t, e, `SELECT label FROM things WHERE id = 2`))
		if len(labelRows.Rows) != 1 {
			t.Fatalf("expected row with id=2")
		}
		if labelRows.Rows[0]["label"] != "updated" {
			t.Fatalf("expected label=updated, got %v", labelRows.Rows[0]["label"])
		}
		wal.Close()
	}()
}

// --------------------------------------------------------------------------
// Concurrent inserts
// --------------------------------------------------------------------------

func TestConcurrentInserts(t *testing.T) {
	e, db := newTestExecutor(t)
	mustExec(t, e, `CREATE TABLE concurrent (id int, val string)`)

	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			query := fmt.Sprintf(`INSERT INTO concurrent VALUES (%d, item%d)`, i, i)
			_, err := e.execute(query)
			if err != nil {
				t.Errorf("goroutine %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	table, err := db.getTable("concurrent")
	if err != nil {
		t.Fatalf("getTable: %v", err)
	}
	table.lock.RLock()
	count := len(table.Rows)
	table.lock.RUnlock()

	if count != n {
		t.Fatalf("expected %d rows, got %d — likely a concurrency bug", n, count)
	}

	// verify no duplicate row IDs
	seen := make(map[string]struct{}, n)
	table.lock.RLock()
	for id := range table.Rows {
		if _, exists := seen[id]; exists {
			t.Errorf("duplicate row ID: %s", id)
		}
		seen[id] = struct{}{}
	}
	table.lock.RUnlock()
}

// --------------------------------------------------------------------------
// Edge cases
// --------------------------------------------------------------------------

func TestSelect_WithLimitAndWhere(t *testing.T) {
	e := setup(t)
	res := asSelect(t, mustExec(t, e, `SELECT * FROM users WHERE age >= 25 LIMIT 2`))
	if len(res.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(res.Rows))
	}
}

func TestSelect_DistinctWithWhere(t *testing.T) {
	e := setup(t)
	// active users ages: Alice=30, Charlie=30, Eve=25 → distinct: 30,25
	res := asSelect(t, mustExec(t, e, `SELECT DISTINCT age FROM users WHERE active = true`))
	if len(res.Rows) != 2 {
		t.Fatalf("expected 2 distinct ages among active users, got %d", len(res.Rows))
	}
}

func TestMultipleTables_Isolated(t *testing.T) {
	e, _ := newTestExecutor(t)
	mustExec(t, e, `CREATE TABLE a (x int)`)
	mustExec(t, e, `CREATE TABLE b (y string)`)
	mustExec(t, e, `INSERT INTO a VALUES (1)`)
	mustExec(t, e, `INSERT INTO b VALUES (hello)`)

	ra := asSelect(t, mustExec(t, e, `SELECT * FROM a`))
	rb := asSelect(t, mustExec(t, e, `SELECT * FROM b`))
	if len(ra.Rows) != 1 || len(rb.Rows) != 1 {
		t.Fatalf("tables should be isolated")
	}
}

func TestNegativeNumber(t *testing.T) {
	e, _ := newTestExecutor(t)
	mustExec(t, e, `CREATE TABLE t (id int, temp int)`)
	mustExec(t, e, `INSERT INTO t VALUES (1, -5)`)
	rows := asSelect(t, mustExec(t, e, `SELECT * FROM t WHERE temp = -5`))
	if len(rows.Rows) != 1 {
		t.Fatalf("expected 1 row with negative number, got %d", len(rows.Rows))
	}
}

func TestFloatStorage(t *testing.T) {
	e, _ := newTestExecutor(t)
	mustExec(t, e, `CREATE TABLE t (id int, price float)`)
	mustExec(t, e, `INSERT INTO t VALUES (1, 19.99)`)
	rows := asSelect(t, mustExec(t, e, `SELECT price FROM t WHERE id = 1`))
	if len(rows.Rows) != 1 {
		t.Fatalf("expected 1 row")
	}
	if rows.Rows[0]["price"] != 19.99 {
		t.Fatalf("expected price=19.99, got %v", rows.Rows[0]["price"])
	}
}
