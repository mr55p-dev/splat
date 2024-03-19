package main

import (
	"log"
	"net/http"
)

func main() {
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
