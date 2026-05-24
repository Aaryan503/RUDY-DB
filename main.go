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

type Item struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type WAL struct {
	OpNumber  int       `json:"opNumber"`
	Operation string    `json:"operation"`
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	Timestamp time.Time `json:"timestamp"`
}

type Snapshot struct {
	LastOpNumber int             `json:"lastOpNumber"`
	Items        map[string]Item `json:"items"`
}

type Store struct {
	mu           sync.RWMutex
	items        map[string]Item
	walFile      *os.File
	lastOpNumber int
}

var store Store

func (s *Store) getAll() []Item {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Item, 0, len(s.items))

	for _, item := range s.items {
		result = append(result, item)
	}
	return result
}

func (s *Store) getItem(key string) (Item, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, exists := s.items[key]
	return item, exists
}

func (s *Store) putItem(item *Item, key string) error {
	op := WAL{
		OpNumber:  s.lastOpNumber + 1,
		Operation: "PUT",
		Key:       key,
		Value:     item.Value,
		Timestamp: time.Now(),
	}
	err := s.appendWAL(op)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item.Key = key
	s.items[key] = *item
	s.lastOpNumber = op.OpNumber
	if s.lastOpNumber%10 == 0 {
		err = s.createSnapshot()
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) deleteItem(key string) (bool, error) {
	s.mu.RLock()
	_, exists := s.items[key]
	s.mu.RUnlock()
	if !exists {
		return false, nil
	}
	op := WAL{
		OpNumber:  s.lastOpNumber + 1,
		Operation: "DELETE",
		Key:       key,
		Timestamp: time.Now(),
	}
	err := s.appendWAL(op)
	if err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, key)
	s.lastOpNumber = op.OpNumber
	if s.lastOpNumber%10 == 0 {
		err = s.createSnapshot()
		if err != nil {
			return false, err
		}
	}
	return true, nil
}

func (s *Store) appendWAL(op WAL) error {
	bytes, err := json.Marshal(op)
	if err != nil {
		return err
	}
	_, err = s.walFile.Write(append(bytes, '\n'))
	if err != nil {
		return err
	}
	return s.walFile.Sync()
}

func (s *Store) createSnapshot() error {
	file, err := os.Create("snapshot.json")
	if err != nil {
		return err
	}
	defer file.Close()
	snap := Snapshot{
		LastOpNumber: s.lastOpNumber,
		Items:        s.items,
	}
	encoder := json.NewEncoder(file)
	err = encoder.Encode(snap)
	if err != nil {
		return err
	}
	err = file.Sync()
	if err != nil {
		return err
	}
	s.walFile.Close()
	os.Create("wal.log")
	wal, err := os.OpenFile(
		"wal.log",
		os.O_CREATE|os.O_RDWR|os.O_APPEND,
		0644,
	)
	if err != nil {
		return err
	}
	s.walFile = wal
	_, err = s.walFile.Seek(0, 0)
	return err
}

func (s *Store) loadSnapshot() error {
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
	s.items = snap.Items
	s.lastOpNumber = snap.LastOpNumber
	return nil
}

func (s *Store) loadWAL() error {
	_, err := s.walFile.Seek(0, 0)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(s.walFile)
	for {
		var op WAL
		err := decoder.Decode(&op)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if op.OpNumber <= s.lastOpNumber {
			continue
		}
		switch op.Operation {
		case "PUT":
			s.items[op.Key] = Item{
				Key:   op.Key,
				Value: op.Value,
			}
		case "DELETE":
			delete(s.items, op.Key)
		}
		s.lastOpNumber = op.OpNumber
	}
	return nil
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
	defer wal.Close()
	store = Store{
		items:   make(map[string]Item),
		walFile: wal,
	}
	err = store.loadSnapshot()
	if err != nil {
		panic(err)
	}
	err = store.loadWAL()
	if err != nil {
		panic(err)
	}
	r := chi.NewRouter()
	r.Get("/items", getAllItems)
	r.Get("/items/{key}", getItem)
	r.Put("/items/{key}", putItem)
	r.Delete("/items/{key}", deleteItem)
	fmt.Println("Server running on :8080")
	http.ListenAndServe(":8080", r)
}

func getAllItems(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(store.getAll())
}

func getItem(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	item, exists := store.getItem(key)
	if !exists {
		http.Error(w, "item not found", 404)
		return
	}
	json.NewEncoder(w).Encode(item)
}

func putItem(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	var item Item
	err := json.NewDecoder(r.Body).Decode(&item)
	if err != nil {
		http.Error(w, "invalid json", 400)
		return
	}
	err = store.putItem(&item, key)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(item)
}

func deleteItem(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	ok, err := store.deleteItem(key)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if !ok {
		http.Error(w, "item not found", 404)
		return
	}
	w.Write([]byte("deleted"))
}
