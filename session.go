package statesman

import (
	"net/http"
)

// Session stores the state of an individual session
// Each session executes a single function that handles several http request.
type Session struct {
	// All HTTP requests are passed to internal HandleFunc. When the HandleFunc
	// returns whatever data was written to the http.ResponseWriter gets sent as
	// a response to the client. This channel is used to block the HandleFunc
	// until the session has completed handling the request.
	handlerGuard chan bool
	// The session will receive the current HTTP request over this channel from
	// one of the internal HandleFuncs.
	httpRequestCh chan *httpRequest
	// The controller that owns this session
	controller *Controller
}

type httpRequest struct {
	w http.ResponseWriter
	r *http.Request
}

func (session *Session) blockHandler() {
	<-session.handlerGuard
}

func (session *Session) unblockHandler() {
	session.handlerGuard <- true
}

// First returns the request and response data for the HTTP request that started the
// session.
func (session *Session) First() (w http.ResponseWriter, r *http.Request) {
	request := <-session.httpRequestCh
	return request.w, request.r
}

// Next returns the request and response data for the named HTTP request
// It blocks until the HTTP request is received.
func (session *Session) Next() (w http.ResponseWriter, r *http.Request) {
	// When the session is asking for the next HTTP request we know that the
	// previous HTTP request has been handled. Allow the previous HandleFunc to
	// finish, which will cause the previous HTTP request's response to be sent
	// to the client.
	session.unblockHandler()

	return session.First()
}
