package server

import (
	"context"
	"log"
	"net"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"time"

	"github.com/usb-wiper/internal/ulid"
)

// contextKey is the type for request-scoped context values.
type contextKey int

const (
	// ctxRequestID is the key for the per-request correlation ULID.
	ctxRequestID contextKey = iota
)

// requestIDFromContext retrieves the correlation ID from the request context.
// Returns empty string if not set.
func requestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(ctxRequestID).(string); ok {
		return id
	}
	return ""
}

// withLogging wraps a handler with request logging and correlation IDs.
// A ULID is generated per request, stored in the context, and included in
// the log line so HTTP logs can be correlated with audit entries.
func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		reqID := ulid.New()
		r = r.WithContext(context.WithValue(r.Context(), ctxRequestID, reqID))

		// Expose the request ID to the client for end-to-end correlation.
		w.Header().Set("X-Request-ID", reqID)

		// Wrap response writer to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)
		log.Printf("[%s] %s %s %d %s", reqID, r.Method, r.URL.Path, wrapped.statusCode, duration.Round(time.Microsecond))
	})
}

// withRecovery wraps a handler with panic recovery.
func withRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic recovered: %v\n%s", rec, debug.Stack())
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"error":"internal server error"}`))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// withCORS wraps a handler with permissive CORS headers for localhost use.
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && isLocalOrigin(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PUT, DELETE")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Max-Age", "86400")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// withOriginCheck rejects state-changing cross-origin requests. Auth is handled by
// an external gateway; this only stops a browser on another site from driving the
// appliance with the operator's own session.
func withOriginCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}

		// Same-origin fetches and non-browser clients (curl, health probes,
		// the external gateway) are fine.
		if secFetchSite := r.Header.Get("Sec-Fetch-Site"); secFetchSite == "same-origin" || secFetchSite == "none" {
			next.ServeHTTP(w, r)
			return
		}

		origin := r.Header.Get("Origin")
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}

		u, err := url.Parse(origin)
		if err == nil && u.Host == r.Host {
			next.ServeHTTP(w, r)
			return
		}

		writeError(w, http.StatusForbidden, "cross-origin request rejected")
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Flush implements http.Flusher by delegating to the underlying ResponseWriter
// if it supports flushing. This is required for SSE streaming to work through
// the middleware chain.
func (rw *responseWriter) Flush() {
	if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// isLocalOrigin checks whether an Origin header value is a literal local or
// private address. Only literal hostnames (localhost) and directly parseable
// IPs are accepted — a host that would need DNS resolution is refused, so an
// attacker-controlled hostname cannot be resolved by the server.
func isLocalOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}

	host := u.Hostname()

	// Allow localhost by name
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}

	// Parse IP and check private/loopback ranges. No resolution of hostnames:
	// anything that is not a literal IP here is refused.
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}

	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}
