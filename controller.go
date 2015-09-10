package statesman

import (
	"fmt"
	"net/http"
	"time"
)

// DefaultTimeout is the default session timeout used by DefaultOptions
var DefaultTimeout time.Duration

// DefaultInvalidSessionHandler is the default function called when a session
// handler endpoint is called without a valid session.
var DefaultInvalidSessionHandler func(http.ResponseWriter, *http.Request)

// The DefaultOptions that are used when NewController is called with a new
// Options
var DefaultOptions Options

func init() {
	DefaultTimeout = time.Minute * 10
	DefaultInvalidSessionHandler = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(fmt.Sprintf("Invalid Session. (%s).", r.URL.Path)))
	}
	DefaultOptions = Options{DefaultTimeout, DefaultInvalidSessionHandler}
}

const (
	statesmanPrefix = "Statesman-"
)

// Struct for (un-)registering sessions
type registration struct {
	sessionKey string
	session    *Session
	register   bool
}

type sessionRequest struct {
	sessionKey      string
	sessionReceiver chan *Session
}

// Controller manages a workflow with potentially several client
// sessions.
type Controller struct {
	options          *Options
	registrationCh   chan *registration
	sessionRequestCh chan *sessionRequest
	sessionCountCh   chan int
	closeCh          chan bool
	openCh           chan bool
}

// Options configures the Controller
type Options struct {
	sessionTimeout        time.Duration
	invalidSessionHandler func(http.ResponseWriter, *http.Request)
}

// NewController constructs a new Controller with the given options.
func NewController(options *Options) *Controller {
	if options == nil {
		options = &DefaultOptions
	}
	if options.sessionTimeout == 0 {
		options.sessionTimeout = DefaultOptions.sessionTimeout
	}

	controller := Controller{
		options,
		make(chan *registration),
		make(chan *sessionRequest),
		make(chan int),
		make(chan bool),
		make(chan bool),
	}

	// Start a service for handling session registrations
	go func() {
		sessions := make(map[string]*Session)
		for {
			select {
			case registration := <-controller.registrationCh:
				// (un-)registration
				if registration.register {
					sessions[registration.sessionKey] = registration.session
				} else {
					delete(sessions, registration.sessionKey)
				}
			case sessionRequest := <-controller.sessionRequestCh:
				// get the session
				session := sessions[sessionRequest.sessionKey]
				sessionRequest.sessionReceiver <- session
			case controller.sessionCountCh <- len(sessions):
				// get the session count
			case <-controller.closeCh:
				// close the controller
				close(controller.openCh)
				return
			case controller.openCh <- true:
				// is open
			}
		}
	}()
	return &controller
}

func (clr *Controller) panicIfClosed() {
	if !<-clr.openCh {
		panic("Controller is closed.")
	}
}

// Close closes the controller. Methods called on a closed controller will
// panic.
func (clr *Controller) Close() error {
	clr.panicIfClosed()
	clr.closeCh <- true
	return nil

}

func (clr *Controller) register(sessionKey string, session *Session) {
	clr.panicIfClosed()
	clr.registrationCh <- &registration{sessionKey, session, true}
}

func (clr *Controller) unregister(sessionKey string) {
	clr.panicIfClosed()
	clr.registrationCh <- &registration{sessionKey, nil, false}
}

func (clr *Controller) session(sessionKey string) *Session {
	clr.panicIfClosed()

	sessionReceiver := make(chan *Session)
	clr.sessionRequestCh <- &sessionRequest{sessionKey, sessionReceiver}

	return <-sessionReceiver
}

func (clr *Controller) sessionCount() int {
	clr.panicIfClosed()
	return <-clr.sessionCountCh
}

// SessionStart returns a handler function to initiate the session handler. The
// Controller.First() method can be called to get the http.ResponseWriter and
// http.Request. The returned handler function can be used with http.HandleFunc
func (clr *Controller) SessionStart(sessionHandler func(s *Session)) func(w http.ResponseWriter, r *http.Request) {
	clr.panicIfClosed()
	sessionInitializer := func(w http.ResponseWriter, r *http.Request) {

		session := Session{make(chan bool), make(chan *httpRequest), clr}

		// Create a session and register it with the session controller
		sessionKey := statesmanPrefix + generateUniqueString(32)

		// Register the session with the controller
		clr.register(sessionKey, &session)

		go func() {
			sessionHandler(&session)

			// The request has been serviced so allow the previous handler
			// function to finish
			session.unblockHandler()

			// Unregister the session from the controller
			clr.unregister(sessionKey)
		}()

		// Set the session cookie before passing the response onto the session
		// to avoid a race condition with the session goroutine
		http.SetCookie(w, generateSessionCookie(sessionKey, "", clr.options.sessionTimeout))

		// Send the initial request to the session (received via First()).
		session.httpRequestCh <- &httpRequest{w, r}

		// Wait for the session to handle the request before returning from this HandleFunc
		session.blockHandler()
	}

	return sessionInitializer
}

// SessionHandler returns a handler function to process within the session
// handler. The Controller.Next() method can be called to get the
// http.ResponseWriter and http.Request. The returned handler function can be
// used with http.HandleFunc
func (clr *Controller) SessionHandler() func(w http.ResponseWriter, r *http.Request) {
	clr.panicIfClosed()
	nextHandler := func(w http.ResponseWriter, r *http.Request) {
		cookie, err := findSessionCookie(r.Cookies())
		// Matching session cookie doesn't exist
		if err != nil {
			clr.options.invalidSessionHandler(w, r)
			return
		}
		sessionKey := cookie.Name

		// Session doesn't exist or has already exited
		session := clr.session(sessionKey)
		if session == nil {
			clr.options.invalidSessionHandler(w, r)
			return
		}

		// Set the session cookie before passing the response onto the session
		// to avoid a race condition with the session goroutine
		http.SetCookie(w, generateSessionCookie(sessionKey, "", clr.options.sessionTimeout))

		// Send this request to the session (received via Next().
		session.httpRequestCh <- &httpRequest{w, r}

		// Block this handler until the session has serviced the current request
		session.blockHandler()
	}
	return nextHandler
}
