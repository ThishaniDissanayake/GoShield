package gateway

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/gin-gonic/gin"
)

// NewReverseProxy creates a reverse proxy that forwards requests to the
// given upstream URL. It preserves the original request path, query
// parameters, headers, and body.
func NewReverseProxy(upstream string) *httputil.ReverseProxy {
	target, err := url.Parse(upstream)
	if err != nil {
		log.Fatalf("❌ Invalid UPSTREAM_URL: %v", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	// Customise the Director to rewrite the request for the upstream.
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = target.Host // forward the upstream Host header
	}

	// Log proxy errors instead of crashing.
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("⚠️  Proxy error: %v", err)
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(`{"error":"bad gateway"}`))
	}

	return proxy
}

// ProxyHandler returns a Gin handler that forwards every request to the
// upstream through the given reverse proxy.
func ProxyHandler(proxy *httputil.ReverseProxy) gin.HandlerFunc {
	return func(c *gin.Context) {
		proxy.ServeHTTP(c.Writer, c.Request)
	}
}
