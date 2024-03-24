package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	http.HandleFunc("/data", func(w http.ResponseWriter, r *http.Request) {
		data, err := os.ReadFile("/volumes/data/file.txt")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
		} else {
			w.Write(data)
			w.Header().Set("Content-Type", "text/plain")
		}
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Println(
			"Request received",
			"method",
			r.Method,
			"path",
			r.URL.Path,
		)
		w.Write([]byte("ok"))
	})
	log.Println("Starting server")
	if err := http.ListenAndServe("0.0.0.0:8080", nil); err != nil {
		log.Println(err.Error())
	}
}
