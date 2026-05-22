package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
)

type Item struct {
	ID   string `json:"id"` //marker for json so that our encoders and decoders recognize which attr is which in json and struct
	Name string `json:"name"`
}

type Store struct {
	mu    sync.RWMutex //apparently we need the Mutex in case multiple users are doing operations together so the data does not get changed/corrupted
	items map[string]Item
}

var store = Store{
	items: make(map[string]Item),
}

func main() {
	r := chi.NewRouter()
	r.Get("/items", getAllItems)
	r.Get("/items/{id}", getItem)
	r.Put("/items/{id}", putItem)
	r.Delete("/items/{id}", deleteItem)
	fmt.Println("Server is running on port 8080 ")
	http.ListenAndServe(":8080", r)
}

func getAllItems(w http.ResponseWriter, r *http.Request) {
	//w is response stream, r is incoming stream. They are passed as arguments here
	//http.Request is a pointer as the Request object is large apparently and better to store the start memory location of it so yeah
	w.Header().Set("Content-Type", "application/json")
	store.mu.RLock()
	defer store.mu.RUnlock() //concurrency control wow (DBMS)
	var result []Item
	for _, item := range store.items {
		result = append(result, item)
	}
	json.NewEncoder(w).Encode(result)
}

func getItem(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	store.mu.RLock()
	defer store.mu.RUnlock()
	item, exists := store.items[id]
	if !exists {
		http.Error(w, "item not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(item)

	//Encoder is reading a struct and then converting it to a json object so it can be written to the response stream. it is "encoding" the item basically
}

func putItem(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var item Item
	err := json.NewDecoder(r.Body).Decode(&item)
	//Decoder takes the incoming stream which is bytes, decodes it to a json object, and then decodes it to a go struct based on the markers we have given it.
	//we pass it by reference because the item is initially empty in memory and then modified based on the decoding
	if err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	item.ID = id
	store.mu.Lock()
	store.items[id] = item
	store.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(item)
}

func deleteItem(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	store.mu.Lock()
	defer store.mu.Unlock()
	_, exists := store.items[id]
	if !exists {
		http.Error(w, "item not found", http.StatusNotFound)
		return
	}
	delete(store.items, id)
	w.Write([]byte("deleted"))
}
