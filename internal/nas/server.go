
// Package nas implements the Nintendo NAS HTTP authentication server.
package nas

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/IrishBruse/mkw-dwc/internal/database"
	"github.com/IrishBruse/mkw-dwc/internal/httpfix"
	"github.com/IrishBruse/mkw-dwc/internal/logging"
)

const (
	nodeHeader   = "wifiappe1"
	authtokenLen = 80
)

// Server serves NAS /ac and /pr endpoints.
type Server struct {
	DB      database.Store
	SvcHost string
	Addr    string
}

// Serve listens until ctx is cancelled.
func (s *Server) Serve(ctx context.Context) error {
	srv := &http.Server{
		Addr:              s.Addr,
		Handler:           s.routes(),
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
	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return err
	}
	logging.For("nas").Infof("listening on %s", s.Addr)
	if err := srv.Serve(httpfix.WrapListener(ln, "nas")); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRoot)
	mux.HandleFunc("/ac", s.handleAC)
	mux.HandleFunc("/pr", s.handlePR)
	return mux
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Header().Set("X-Organization", "Nintendo")
	w.Header().Set("Server", "BigIP")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "ok")
}

func (s *Server) handleAC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	post, err := readPost(r)
	if err != nil {
		logging.For("nas").Warnf("bad POST")
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	post["ipaddr"] = clientIP(r)

	action := strings.ToLower(post["action"])
	logging.For("nas").Infof("%s userid=%s gamecd=%s ip=%s", action, post["userid"], post["gamecd"], post["ipaddr"])
	var ret map[string]string
	switch action {
	case "acctcreate":
		ret = s.acctCreate(post)
	case "login":
		ret = s.acLogin(post)
	case "svcloc":
		ret = s.acSvcLoc(r, post)
	default:
		logging.For("nas").Warnf("unknown action: %s", action)
		ret = map[string]string{}
	}

	ret["datetime"] = time.Now().Format("20060102150405")
	writeNASResponse(w, dictToQS(ret))
}

func (s *Server) handlePR(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	post, err := readPost(r)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	_ = post

	words := len(strings.Split(post["words"], "\t"))
	wordsRet := strings.Repeat("0", words)
	ret := map[string]string{
		"prwords":  wordsRet,
		"returncd": "000",
		"datetime": time.Now().Format("20060102150405"),
	}
	for _, suffix := range "ACEJKP" {
		ret["prwords"+string(suffix)] = wordsRet
	}
	writeNASResponse(w, dictToQS(ret))
}

func (s *Server) acctCreate(post map[string]string) map[string]string {
	if s.DB.IsBanned(banGameCD(post["gamecd"]), post["ipaddr"]) {
		logging.For("nas").Warnf("acctcreate banned gamecd=%s ip=%s", post["gamecd"], post["ipaddr"])
		return map[string]string{
			"retry":    "1",
			"returncd": "3913",
			"locator":  "gamespy.com",
			"reason":   "User banned.",
		}
	}
	return map[string]string{
		"retry":    "0",
		"returncd": "002",
		"userid":   s.DB.GetNextAvailableUserid(),
	}
}

func (s *Server) acLogin(post map[string]string) map[string]string {
	if s.DB.IsBanned(banGameCD(post["gamecd"]), post["ipaddr"]) {
		logging.For("nas").Warnf("login banned gamecd=%s ip=%s", post["gamecd"], post["ipaddr"])
		return map[string]string{
			"retry":    "1",
			"returncd": "3914",
			"locator":  "gamespy.com",
			"reason":   "User banned.",
		}
	}

	challenge := randomAlphanumeric(8)
	data := copyMap(post)
	data["challenge"] = challenge

	authtoken, err := s.generateAuthtoken(post["userid"], data)
	if err != nil {
		logging.For("nas").Errorf("generateAuthtoken failed for userid=%s: %v", post["userid"], err)
		return map[string]string{
			"retry":    "1",
			"returncd": "9999",
			"locator":  "gamespy.com",
			"reason":   "Server error.",
		}
	}

	return map[string]string{
		"retry":     "0",
		"returncd":  "001",
		"locator":   "gamespy.com",
		"challenge": challenge,
		"token":     authtoken,
	}
}

func (s *Server) acSvcLoc(r *http.Request, post map[string]string) map[string]string {
	ret := map[string]string{
		"retry":      "0",
		"returncd":   "007",
		"statusdata": "Y",
	}

	data := copyMap(post)
	authtoken, err := s.generateAuthtoken(post["userid"], data)
	if err != nil {
		logging.For("nas").Errorf("generateAuthtoken failed for userid=%s: %v", post["userid"], err)
		return ret
	}

	svc, ok := post["svc"]
	if !ok {
		return ret
	}

	svchost := s.SvcHost
	if svchost == "" {
		svchost = r.Host
	}
	if idx := strings.Index(svchost, ","); idx >= 0 {
		svchost = svchost[:idx]
	}

	switch svc {
	case "9000", "9001":
		ret["svchost"] = svchost
		if svc == "9000" {
			ret["token"] = authtoken
		} else {
			ret["servicetoken"] = authtoken
		}
	case "0000":
		ret["servicetoken"] = authtoken
		ret["svchost"] = "n/a"
	default:
		ret["svchost"] = "n/a"
		ret["servicetoken"] = authtoken
	}
	return ret
}

func (s *Server) generateAuthtoken(userid string, data map[string]string) (string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	for range 16 {
		token := "NDS" + randomFromCharset(authtokenLen, alphabet)
		if err := s.DB.StoreNasLogin(userid, token, data); err == nil {
			return token, nil
		}
	}
	return "", fmt.Errorf("nas: could not allocate authtoken")
}

func readPost(r *http.Request) (map[string]string, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	return qsToDict(string(body)), nil
}

func writeNASResponse(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("NODE", nodeHeader)
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, body)
}

func clientIP(r *http.Request) string {
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			return r.RemoteAddr
		}
		return host
	}
	if idx := strings.Index(ip, ","); idx >= 0 {
		ip = ip[:idx]
	}
	return strings.TrimSpace(ip)
}

func banGameCD(gamecd string) string {
	if len(gamecd) == 0 {
		return gamecd
	}
	return gamecd[:len(gamecd)-1]
}

func qsToDict(s string) map[string]string {
	ret := make(map[string]string)
	for _, pair := range strings.Split(s, "&") {
		if pair == "" {
			continue
		}
		key, value, ok := strings.Cut(pair, "=")
		if !ok {
			ret[key] = ""
			continue
		}
		decoded, err := decodeNASValue(value)
		if err != nil {
			ret[key] = value
			continue
		}
		ret[key] = decoded
	}
	return ret
}

func decodeNASValue(v string) (string, error) {
	unquoted, err := url.QueryUnescape(v)
	if err != nil {
		unquoted = v
	}
	unquoted = strings.NewReplacer("*", "=", "?", "/", ">", "+", "-", "/").Replace(unquoted)
	raw, err := base64.StdEncoding.DecodeString(unquoted)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func dictToQS(d map[string]string) string {
	keys := make([]string, 0, len(d))
	for k := range d {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		enc := base64.StdEncoding.EncodeToString([]byte(d[k]))
		enc = strings.ReplaceAll(enc, "=", "*")
		parts = append(parts, k+"="+enc)
	}
	return strings.Join(parts, "&") + "\r\n"
}

func copyMap(src map[string]string) map[string]string {
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func randomAlphanumeric(n int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	return randomFromCharset(n, alphabet)
}

func randomFromCharset(n int, alphabet string) string {
	if n <= 0 {
		return ""
	}
	var b strings.Builder
	b.Grow(n)
	max := big.NewInt(int64(len(alphabet)))
	for range n {
		i, err := rand.Int(rand.Reader, max)
		if err != nil {
			b.WriteByte(alphabet[0])
			continue
		}
		b.WriteByte(alphabet[i.Int64()])
	}
	return b.String()
}
