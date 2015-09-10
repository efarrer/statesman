package statesman_test

import (
	"fmt"
	"log"
	"net/http"
	"statesman"
)

const (
	START = "/start"
	COUNT = "/count"
	QUIT  = "/quit"
)

func countingHandler(session *statesman.Session) {
	w, _ := session.First()
	fmt.Fprintf(w, "Hello World")

	count := 0
	for {
		w, r := session.Next()
		if r.URL.Path == COUNT {
			fmt.Fprintf(w, fmt.Sprintf("%d", count))
			count++
		} else {
			fmt.Fprintf(w, "Goodbye World")
			return
		}
	}
}

func ExampleController() {
	sessionController := statesman.NewController(nil)
	http.HandleFunc(START, sessionController.SessionStart(countingHandler))
	http.HandleFunc(COUNT, sessionController.SessionHandler())
	http.HandleFunc(QUIT, sessionController.SessionHandler())
	log.Fatal(http.ListenAndServe(":8080", nil))
}
