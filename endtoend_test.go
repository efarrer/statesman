package statesman

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/cookiejar"
	"testing"
)

func assertNoError(err error) {
	if err != nil {
		panic(err)
	}
}

func newClient() *http.Client {
	jar, err := cookiejar.New(nil)
	assertNoError(err)
	return &http.Client{Jar: jar}
}

func listenAndServeBackground(mux *http.ServeMux) (*net.Listener, string) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	assertNoError(err)
	go func() {
		server := http.Server{Handler: mux}
		server.Serve(l)
	}()

	return &l, "http://" + l.Addr().String()
}

func getAndExpect(client *http.Client, url, expectedResult string, t *testing.T) {
	res, err := client.Get(url)
	assertNoError(err)
	bytes, err := ioutil.ReadAll(res.Body)
	assertNoError(err)
	if string(bytes) != expectedResult {
		t.Fatalf("Got unexpected response from %s. Expecting \"%s\" got \"%s\"\n", url, expectedResult, string(bytes))
	}
}

func Test_executeSessionWithFirstNextData(t *testing.T) {
	firstPath := "/first"
	nextPath := "/next"
	closePath := "/close"

	firstData := "First"
	nextDatas := []string{"This", "is", "the", "next", "data"}
	closeData := "closed"

	sessionHandler := func(s *Session) {
		w, r := s.First()
		if r.URL.Path != firstPath {
			t.Fatalf("Got unexpected path. Expecting %s got %s\n", firstPath, r.URL.Path)
		}
		fmt.Fprintf(w, firstData)

		for _, nextData := range nextDatas {
			w, r = s.Next()
			if r.URL.Path != nextPath {
				t.Fatalf("Got unexpected path. Expecting %s got %s\n", nextPath, r.URL.Path)
			}
			fmt.Fprintf(w, nextData)
		}
		w, r = s.Next()
		if r.URL.Path != closePath {
			t.Fatalf("Got unexpected path. Expecting %s got %s\n", nextPath, r.URL.Path)
		}
		fmt.Fprintf(w, closeData)
	}
	sc := NewController(&DefaultOptions)
	mux := http.NewServeMux()
	mux.HandleFunc(firstPath, sc.SessionStart(sessionHandler))
	mux.HandleFunc(nextPath, sc.SessionHandler())
	mux.HandleFunc(closePath, sc.SessionHandler())
	l, listenURL := listenAndServeBackground(mux)
	defer (*l).Close()

	client := newClient()
	getAndExpect(client, listenURL+firstPath, firstData, t)

	for _, nextData := range nextDatas {
		getAndExpect(client, listenURL+nextPath, nextData, t)
	}

	getAndExpect(client, listenURL+closePath, closeData, t)
}

func Test_executeSessionWithMultipleClients(t *testing.T) {
	firstPath := "/first"
	closePath := "/close"

	counterCh := make(chan int)
	go func() {
		for i := 0; true; i++ {
			counterCh <- i
		}
	}()

	sessionHandler := func(s *Session) {
		w, r := s.First()
		if r.URL.Path != firstPath {
			t.Fatalf("Got unexpected path. Expecting %s got %s\n", firstPath, r.URL.Path)
		}
		fmt.Fprintf(w, "%d", <-counterCh)

		w, r = s.Next()
		if r.URL.Path != closePath {
			t.Fatalf("Got unexpected path. Expecting %s got %s\n", closePath, r.URL.Path)
		}
		fmt.Fprintf(w, "%d", <-counterCh)
	}
	sc := NewController(&DefaultOptions)
	mux := http.NewServeMux()
	mux.HandleFunc(firstPath, sc.SessionStart(sessionHandler))
	mux.HandleFunc(closePath, sc.SessionHandler())
	l, listenURL := listenAndServeBackground(mux)
	defer (*l).Close()

	client0 := newClient()
	getAndExpect(client0, listenURL+firstPath, "0", t)

	client1 := newClient()
	getAndExpect(client1, listenURL+firstPath, "1", t)

	getAndExpect(client1, listenURL+closePath, "2", t)
	getAndExpect(client0, listenURL+closePath, "3", t)
}

func Benchmark_Controller(b *testing.B) {
	firstPath := "/first"
	loopPath := "/next"
	closePath := "/close"
	firstData := "First"
	loopData := "Next"
	closeData := "Close"
	sessionHandler := func(s *Session) {
		w, _ := s.First()
		fmt.Fprintf(w, firstData)
		for {
			w, r := s.Next()
			if r.URL.Path == loopPath {
				fmt.Fprintf(w, loopData)
			} else {
				fmt.Fprintf(w, closeData)
				return
			}
		}
	}
	sc := NewController(nil)
	mux := http.NewServeMux()
	mux.HandleFunc(firstPath, sc.SessionStart(sessionHandler))
	mux.HandleFunc(loopPath, sc.SessionHandler())
	mux.HandleFunc(closePath, sc.SessionHandler())
	l, listenURL := listenAndServeBackground(mux)
	defer (*l).Close()
	client := newClient()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i == 0 {
			resp, err := client.Get(listenURL + firstPath)
			assertNoError(err)
			resp.Body.Close()
		} else if i+1 == b.N {
			resp, err := client.Get(listenURL + closePath)
			assertNoError(err)
			resp.Body.Close()
		} else {
			resp, err := client.Get(listenURL + loopPath)
			assertNoError(err)
			resp.Body.Close()
		}
	}
}
