package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
)

func main() {
	http.HandleFunc("/weather/", func(w http.ResponseWriter, r *http.Request) {
		city := strings.TrimPrefix(r.URL.Path, "/weather/")
		log.Printf("Weather service: GET /weather/%s", city)

		data := map[string]string{
			"city":        city,
			"temperature": "22°C",
			"condition":   fmt.Sprintf("Sunny in %s", strings.Title(city)),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(data)
	})
	log.Println("Starting Weather Service on :9001")
	http.ListenAndServe(":9001", nil)
}