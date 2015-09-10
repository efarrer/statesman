package statesman

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type testResponseWriter struct {
	header http.Header
	status int
}

func (tsr *testResponseWriter) Header() http.Header {
	if tsr.header == nil {
		tsr.header = http.Header{}
	}
	return tsr.header
}

func (tsr *testResponseWriter) Write([]byte) (int, error) {
	return 0, nil
}

func (tsr *testResponseWriter) WriteHeader(status int) {
	tsr.status = status
}

type testClient struct {
	w *testResponseWriter
	r *http.Request
}

func newTestClient() *testClient {
	tc := &testClient{}
	tc.init()
	return tc
}

func (ts *testClient) init() {
	ts.w = &testResponseWriter{}
	ts.r = &http.Request{URL: &url.URL{Path: ""}, Header: http.Header{}}
}

func (ts *testClient) get(handler func(w http.ResponseWriter, r *http.Request)) chan bool {
	sessionKey := ""
	if ts.w != nil {
		cookies := ts.w.Header()["Set-Cookie"]
		if len(cookies) != 0 {
			cookie := cookies[0]
			sessionKey = strings.Split(cookie, "=")[0]
		}
	}
	ts.init()
	if sessionKey != "" {
		ts.r.AddCookie(generateSessionCookie(sessionKey, "", time.Minute*10))
	}
	doneCh := make(chan bool)
	go func() {
		handler(ts.w, ts.r)
		doneCh <- true
	}()
	return doneCh
}

func TestBlockHandler_WaitsForCallToUnblockHandler(t *testing.T) {
	session := Session{handlerGuard: make(chan bool)}
	sequencerA := make(chan int, 4)
	sequencerB := make(chan int, 4)

	go func() {
		sequencerA <- 0
		session.unblockHandler()
		sequencerB <- 1
	}()
	sequencerB <- 0
	session.blockHandler()
	sequencerA <- 1

	if <-sequencerA != 0 {
		t.Fatalf("Didn't get a 0 from A\n")
	}
	if <-sequencerA != 1 {
		t.Fatalf("Didn't get a 1 from A\n")
	}
	if <-sequencerB != 0 {
		t.Fatalf("Didn't get a 0 from B\n")
	}
	if <-sequencerB != 1 {
		t.Fatalf("Didn't get a 1 from B\n")
	}
}

func TestFirst_RetrievesFirstRequest(t *testing.T) {
	tc := newTestClient()
	session := &Session{httpRequestCh: make(chan *httpRequest)}

	go func() {
		session.httpRequestCh <- &httpRequest{tc.w, tc.r}
	}()

	w, r := session.First()

	if w != tc.w {
		t.Fatalf("Didn't get the expected ResponseWriter\n")
	}

	if r != tc.r {
		t.Fatalf("Didn't get the expected Request\n")
	}
}

func TestNext_NotifiesThatPreviousRequestHasBeenServiced(t *testing.T) {
	tc := newTestClient()
	session := Session{
		handlerGuard:  make(chan bool),
		httpRequestCh: make(chan *httpRequest),
	}
	sequencerA := make(chan int, 4)

	go func() {
		session.blockHandler()
		sequencerA <- 0
		session.httpRequestCh <- &httpRequest{tc.w, tc.r}
	}()

	session.Next()
	if 0 != <-sequencerA {
		t.Fatalf("Next didn't notify previous request that it was serviced\n")
	}
}

func TestNext_ReceivesNextRequest(t *testing.T) {
	tc := newTestClient()
	session := Session{
		handlerGuard:  make(chan bool),
		httpRequestCh: make(chan *httpRequest),
	}

	go func() {
		session.blockHandler()
		session.httpRequestCh <- &httpRequest{tc.w, tc.r}
	}()

	w, r := session.Next()

	if w != tc.w {
		t.Fatalf("Didn't get the expected ResponseWriter\n")
	}

	if r != tc.r {
		t.Fatalf("Didn't get the expected Request\n")
	}
}
