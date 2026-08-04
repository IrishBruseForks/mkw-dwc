
// Package proxy provides a host-based HTTP reverse proxy for Nintendo NAS domains.
package proxy

import (
	"context"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/IrishBruse/mkw-dwc/internal/logging"
)

var nasHosts = map[string]struct{}{
	"naswii.nintendowifi.net": {},
	"nas.nintendowifi.net":    {},
}

// Serve listens on bindAddr and forwards NAS hostnames to nasBackendURL.
func Serve(ctx context.Context, bindAddr string, nasBackendURL string) error {
	target, err := url.Parse(nasBackendURL)
	if err != nil {
		return err
	}

	nasProxy := httputil.NewSingleHostReverseProxy(target)
	origDirector := nasProxy.Director
	nasProxy.Director = func(req *http.Request) {
		origDirector(req)
		req.Host = target.Host
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		host := strings.ToLower(stripPort(r.Host))
		if _, ok := nasHosts[host]; ok {
			logging.For("proxy").Infof("forward %s %s -> backend", r.Method, r.Host+r.URL.Path)
			nasProxy.ServeHTTP(w, r)
			return
		}
		logging.For("proxy").Warnf("unhandled host %q", host)
		http.Error(w, "unhandled host", http.StatusNotFound)
	})

	srv := &http.Server{
		Addr:              bindAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	logging.For("proxy").Infof("listening on %s forwarding to %s", bindAddr, nasBackendURL)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func stripPort(host string) string {
	if i := strings.Index(host, ":"); i >= 0 {
		return host[:i]
	}
	return host
}
