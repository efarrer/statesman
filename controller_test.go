package statesman

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestNewController_AcceptsNilOptions(t *testing.T) {
	crl := NewController(nil)
	if crl.options != &DefaultOptions {
		t.Fatalf("Nil options didn't use default options\n")
	}
}

func TestNewController_AcceptsDefaultOptions(t *testing.T) {
	crl := NewController(&DefaultOptions)
	if crl.options != &DefaultOptions {
		t.Fatalf("DefaultOptions didn't get set\n")
	}
}

func TestControllerClose_ShutsDownControllerService(t *testing.T) {
	crl := NewController(nil)
	err := crl.Close()
	if err != nil {
		t.Fatalf("Closing session shouldn't have returned an err %v\n", err)
	}
}

func TestControllerClose_MethodsPanicIfClosed(t *testing.T) {
	crl := NewController(nil)
	crl.Close()
	funcs := []func(){
		func() { crl.Close() },
		func() { crl.register("", nil) },
		func() { crl.unregister("") },
		func() { crl.session("") },
		func() { crl.sessionCount() },
		func() { crl.SessionStart(func(*Session) {}) },
		func() { crl.SessionHandler() },
	}

	expectedError := "Controller is closed."
	for _, fn := range funcs {
		func() {
			defer func() {
				if r := recover(); r != expectedError {
					t.Fatalf("register should panic with \"%s\", got \"%s\"\n", expectedError, r)
				}
			}()
			fn()
		}()
	}
}

func TestSession_ReturnsARegisteredSession(t *testing.T) {
	crl := NewController(nil)
	defer crl.Close()

	sessionKey := "some session key"
	handlerGuard := make(chan bool)

	session := &Session{handlerGuard: handlerGuard}

	crl.register(sessionKey, session)

	if session != crl.session(sessionKey) {
		t.Fatalf("Expected to get session got %v\n", session)
	}
}

func TestRegister_IncrementsTheSessionCount(t *testing.T) {
	sessionKey := "some session"
	crl := NewController(nil)
	before := crl.sessionCount()
	crl.register(sessionKey, nil)
	if before != crl.sessionCount()-1 {
		t.Fatalf("Register doesn't incrlement the session count")
	}
}

func TestUnregister_DecrementsTheSessionCount(t *testing.T) {
	sessionKey := "some session"
	crl := NewController(nil)
	crl.register(sessionKey, nil)
	before := crl.sessionCount()
	crl.unregister(sessionKey)
	if before != crl.sessionCount()+1 {
		t.Fatalf("Unregister doesn't decrlement the session count")
	}
}

func TestSessionStart_HandlerRegistersThenUnregistersNewSession(t *testing.T) {
	crl := NewController(nil)
	sessionFinished := make(chan bool)
	handler := crl.SessionStart(func(s *Session) {
		defer func() { sessionFinished <- true }()
		if 1 != crl.sessionCount() {
			t.Fatalf("SessionStart didn't register session\n")
		}
		s.First()
	})

	tc := newTestClient()
	tc.get(handler)
	<-sessionFinished

	// There's a race condition so busy wait until the session has been
	// unregistered
	for i := 0; i != 100; i++ {
		if 0 == crl.sessionCount() {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if 0 != crl.sessionCount() {
		t.Fatalf("SessionStart didn't unregister session\n")
	}
}

func TestSessionStart_CallsSessionFunction(t *testing.T) {
	crl := NewController(nil)
	sessionCalled := make(chan bool)
	handler := crl.SessionStart(func(s *Session) {
		defer func() { sessionCalled <- true }()
		s.First()
	})

	tc := newTestClient()
	tc.get(handler)

	if !<-sessionCalled {
		t.Fatalf("Session wasn't called\n")
	}
}

func TestSessionStart_SetsSessionCookieWithCorrectExpiration(t *testing.T) {
	timeout := time.Minute * 20
	options := &Options{timeout, DefaultInvalidSessionHandler}
	crl := NewController(options)
	sessionFinished := make(chan bool)
	handler := crl.SessionStart(func(s *Session) {
		defer func() { sessionFinished <- true }()
		w, _ := s.First()
		if !strings.HasPrefix(w.Header()["Set-Cookie"][0], statesmanPrefix) {
			t.Fatalf("SessionStart didn't set cookie\n")
		}
		expectedTime := time.Now().Add(timeout).UTC().Format(time.RFC1123)
		if !strings.Contains(w.Header()["Set-Cookie"][0], expectedTime) {
			t.Fatalf("SessionStart didn't set correct timeout\n")
		}
	})

	tc := newTestClient()
	tc.get(handler)

	<-sessionFinished
}

func TestSessionStart_SendsRequestToSession(t *testing.T) {
	crl := NewController(nil)
	tc := newTestClient()
	sessionFinished := make(chan bool)
	handler := crl.SessionStart(func(s *Session) {
		defer func() { sessionFinished <- true }()
		w, r := s.First()

		if w != tc.w {
			t.Fatalf("Got unexpected ResponseWriter\n")
		}
		if r != tc.r {
			t.Fatalf("Got unexpected Request\n")
		}
	})

	tc.get(handler)

	<-sessionFinished
}

func TestSessionStart_BlocksHandlerFunctionUntilSessionFinishes(t *testing.T) {
	crl := NewController(nil)
	finishedFirst := make(chan string)
	handler := crl.SessionStart(func(s *Session) {
		s.First()
		time.Sleep(10 * time.Millisecond)
		finishedFirst <- "session"
	})

	tc := newTestClient()
	go func() {
		<-tc.get(handler)
		finishedFirst <- "handler"
	}()

	if "session" != <-finishedFirst {
		t.Fatalf("Session should have finished first\n")
	}
	if "handler" != <-finishedFirst {
		t.Fatalf("Handler should have finished second\n")
	}
}

func TestSessionHandler_ReturnsForbiddenIfHandlerCalledWithoutSession(t *testing.T) {
	crl := NewController(nil)
	handler := crl.SessionHandler()
	tc := newTestClient()
	<-tc.get(handler)

	if tc.w.status != http.StatusForbidden {
		t.Fatalf("Should have gotten StatusForbidden\n")
	}
}

func TestSessionHandler_CallsCustomInvalidSessionHandler(t *testing.T) {
	customInvalidSessionCalledCh := make(chan bool)
	customHandler := func(w http.ResponseWriter, r *http.Request) {
		go func() { customInvalidSessionCalledCh <- true }()
		DefaultInvalidSessionHandler(w, r)
	}
	crl := NewController(&Options{DefaultTimeout, customHandler})
	handler := crl.SessionHandler()
	tc := newTestClient()
	<-tc.get(handler)

	if !<-customInvalidSessionCalledCh {
		t.Fatalf("Custom invalid session handler wasn't called\n")
	}
}

func TestSessionHandler_SetsSessionCookieWithCorrectExpiration(t *testing.T) {
	timeout := time.Minute * 20
	options := &Options{timeout, DefaultInvalidSessionHandler}
	crl := NewController(options)
	sessionFinished := make(chan bool)
	firstHandler := crl.SessionStart(func(s *Session) {
		defer func() { sessionFinished <- true }()
		_, _ = s.First()
		w, _ := s.Next()
		if !strings.HasPrefix(w.Header()["Set-Cookie"][0], statesmanPrefix) {
			t.Fatalf("SessionStart didn't set cookie\n")
		}
		expectedTime := time.Now().Add(timeout).UTC().Format(time.RFC1123)
		if !strings.Contains(w.Header()["Set-Cookie"][0], expectedTime) {
			t.Fatalf("SessionStart didn't set correct timeout\n")
		}
	})
	nextHandler := crl.SessionHandler()

	go func() {
		tc := newTestClient()
		<-tc.get(firstHandler)
		<-tc.get(nextHandler)
	}()

	<-sessionFinished
}

func TestSessionHandler_CallsCustomInvalidSessionHandlerIfSessionNotStarted(t *testing.T) {
	customInvalidSessionCalledCh := make(chan bool)
	customHandler := func(w http.ResponseWriter, r *http.Request) {
		go func() { customInvalidSessionCalledCh <- true }()
		DefaultInvalidSessionHandler(w, r)
	}
	crl := NewController(&Options{DefaultTimeout, customHandler})
	nextHandler := crl.SessionHandler()

	go func() {
		tc := newTestClient()
		sessionKey := statesmanPrefix + generateUniqueString(32)
		tc.r.AddCookie(generateSessionCookie(sessionKey, "", crl.options.sessionTimeout))
		tc.get(nextHandler)
	}()

	if !<-customInvalidSessionCalledCh {
		t.Fatalf("Custom invalid session handler wasn't called\n")
	}
}

func TestSessionHandler_SendsRequestToSession(t *testing.T) {
	crl := NewController(nil)
	tc := newTestClient()
	sessionFinished := make(chan bool)
	firstHandler := crl.SessionStart(func(s *Session) {
		defer func() { sessionFinished <- true }()
		w, r := s.First()
		if w != tc.w {
			t.Fatalf("Got unexpected ResponseWriter\n")
		}
		if r != tc.r {
			t.Fatalf("Got unexpected Request\n")
		}

		w, r = s.Next()

		if w != tc.w {
			t.Fatalf("Got unexpected ResponseWriter\n")
		}
		if r != tc.r {
			t.Fatalf("Got unexpected Request\n")
		}
	})
	nextHandler := crl.SessionHandler()

	go func() {
		<-tc.get(firstHandler)
		<-tc.get(nextHandler)
	}()

	<-sessionFinished
}

func TestSessionHandler_BlocksHandlerFunctionUntilSessionFinishes(t *testing.T) {
	crl := NewController(nil)
	finishedFirst := make(chan string)
	firstHandler := crl.SessionStart(func(s *Session) {
		s.First()
		s.Next()
		time.Sleep(10 * time.Millisecond)
		finishedFirst <- "session"
	})
	nextHandler := crl.SessionHandler()

	go func() {
		tc := newTestClient()
		<-tc.get(firstHandler)
		<-tc.get(nextHandler)
		finishedFirst <- "handler"
	}()

	if "session" != <-finishedFirst {
		t.Fatalf("Session should have finished first\n")
	}
	if "handler" != <-finishedFirst {
		t.Fatalf("Handler should have finished second\n")
	}
}
