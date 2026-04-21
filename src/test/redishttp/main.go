package main

import (
	"context"
	"net/http"

	"github.com/redis/go-redis/v9"
)

var ctx = context.Background()

var rdb = redis.NewClient(&redis.Options{
	Addr: "host.docker.internal:6379",
})

func write(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	value := r.URL.Query().Get("value")

	err := rdb.Set(ctx, key, value, 0).Err()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.Write([]byte("ok"))
}

func main() {
	http.HandleFunc("/write", write)
	http.ListenAndServe(":8081", nil)
}
