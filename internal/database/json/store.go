// Package json persists GPCM profile and NAS login data as JSON files.
package json

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/IrishBruse/mkw-dwc/internal/database"
	"github.com/IrishBruse/mkw-dwc/internal/logging"
)

var storeLog = logging.For("store")

var _ database.Store = (*Store)(nil)

type userRecord struct {
	ProfileID  int64  `json:"profileid"`
	UserID     string `json:"userid"`
	Password   string `json:"password"`
	Gsbrcd     string `json:"gsbrcd"`
	Email      string `json:"email"`
	Uniquenick string `json:"uniquenick"`
	PID        string `json:"pid"`
	Lon        string `json:"lon"`
	Lat        string `json:"lat"`
	Loc        string `json:"loc"`
	Firstname  string `json:"firstname"`
	Lastname   string `json:"lastname"`
	Stat       string `json:"stat"`
	PartnerID  string `json:"partnerid"`
	Console    int    `json:"console"`
	Csnum      string `json:"csnum"`
	Cfc        string `json:"cfc"`
	Bssid      string `json:"bssid"`
	Devname    string `json:"devname"`
	Birth      string `json:"birth"`
	GameID     string `json:"gameid"`
	Enabled    int    `json:"enabled"`
	Zipcode    string `json:"zipcode"`
	Aim        string `json:"aim"`
}

type sessionRecord struct {
	Session     string `json:"session"`
	ProfileID   int64  `json:"profileid"`
	LoginTicket string `json:"loginticket"`
}

type nasLoginRecord struct {
	UserID    string            `json:"userid"`
	AuthToken string            `json:"authtoken"`
	Data      map[string]string `json:"data"`
}

type banRecord struct {
	GameID string `json:"gameid"`
	IPAddr string `json:"ipaddr"`
}

// Store is a file-backed GPCM store using JSON under a data directory.
type Store struct {
	dir string
	mu  sync.Mutex

	users     []userRecord
	sessions  []sessionRecord
	nasLogins []nasLoginRecord
	banned    []banRecord
}

// Open loads (or creates) a JSON store rooted at dir.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("json: mkdir %q: %w", dir, err)
	}
	s := &Store{dir: dir}
	if err := s.loadAll(); err != nil {
		return nil, err
	}
	return s, nil
}

// Close is a no-op for the JSON database.
func (s *Store) Close() error {
	return nil
}

// Initialize ensures the data directory and empty JSON files exist.
func (s *Store) Initialize() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.persistAllLocked()
}

// GetNextAvailableUserid returns the next 13-digit zero-padded userid.
//
// Matches dwc_network_server_emulator: max(users.userid)+1, or 2 when empty
// (0 is Dolphin's sentinel). Not persisted until a users.json row exists, so
// concurrent acctcreate before GPCM login can collide like the reference.
func (s *Store) GetNextAvailableUserid() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	var maxUser int64
	found := false
	for _, u := range s.users {
		n, err := strconv.ParseInt(u.UserID, 10, 64)
		if err != nil {
			continue
		}
		if !found || n > maxUser {
			maxUser = n
			found = true
		}
	}
	if !found {
		return "0000000000002"
	}
	return fmt.Sprintf("%013d", maxUser+1)
}

// IsBanned reports whether gamecd/ipaddr is present in the ban list.
func (s *Store) IsBanned(gamecd, ipaddr string) bool {
	gameid := database.BanGameID(gamecd)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, b := range s.banned {
		if b.GameID == gameid && b.IPAddr == ipaddr {
			return true
		}
	}
	return false
}

// StoreNasLogin inserts or updates NAS login data for a userid.
func (s *Store) StoreNasLogin(userid, authtoken string, data map[string]string) error {
	stored := make(map[string]string, len(data))
	for k, v := range data {
		stored[k] = v
	}
	if v, ok := stored["devname"]; ok {
		stored["devname"] = database.GamespyBase64Encode([]byte(v))
	}
	if v, ok := stored["ingamesn"]; ok {
		stored["ingamesn"] = database.GamespyBase64Encode([]byte(v))
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, login := range s.nasLogins {
		if login.AuthToken == authtoken && login.UserID != userid {
			storeLog.Warnf("authtoken collision userid=%s", userid)
			return fmt.Errorf("json: authtoken collision")
		}
	}

	// Match dwc_network_server_emulator: one nas_logins row per userid.
	// A later login replaces the authtoken for that userid.
	for i := range s.nasLogins {
		if s.nasLogins[i].UserID == userid {
			s.nasLogins[i].AuthToken = authtoken
			s.nasLogins[i].Data = stored
			storeLog.Debugf("nas login upsert userid=%s", userid)
			return s.persistFileLocked("nas_logins.json", s.nasLogins)
		}
	}
	s.nasLogins = append(s.nasLogins, nasLoginRecord{
		UserID:    userid,
		AuthToken: authtoken,
		Data:      stored,
	})
	storeLog.Debugf("nas login insert userid=%s", userid)
	return s.persistFileLocked("nas_logins.json", s.nasLogins)
}

// GetNasLogin returns stored NAS login data for an authtoken.
func (s *Store) GetNasLogin(authtoken string) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, login := range s.nasLogins {
		if login.AuthToken == authtoken {
			out := make(map[string]string, len(login.Data))
			for k, v := range login.Data {
				out[k] = v
			}
			return out, nil
		}
	}
	return nil, nil
}

// GetProfile returns public profile fields for profileID.
func (s *Store) GetProfile(profileID int64) (*database.Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, u := range s.users {
		if u.ProfileID != profileID {
			continue
		}
		return &database.Profile{
			ProfileID:  u.ProfileID,
			UserID:     u.UserID,
			Email:      u.Email,
			Uniquenick: u.Uniquenick,
			PID:        u.PID,
			Lon:        u.Lon,
			Lat:        u.Lat,
			Loc:        u.Loc,
			Firstname:  u.Firstname,
			Lastname:   u.Lastname,
		}, nil
	}
	return nil, fmt.Errorf("json: profile %d not found", profileID)
}

// UpdateProfile applies allowed field updates to profileID.
func (s *Store) UpdateProfile(profileID int64, fields map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx := -1
	for i, u := range s.users {
		if u.ProfileID == profileID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("json: profile %d not found", profileID)
	}

	u := &s.users[idx]
	changed := false
	for key, value := range fields {
		switch key {
		case "firstname":
			u.Firstname = value
			changed = true
		case "lastname":
			u.Lastname = value
			changed = true
		default:
			// Match dwc_network_server_emulator gs_database.update_profile allowlist.
			return fmt.Errorf("json: unsupported profile field %q", key)
		}
	}
	if !changed {
		return nil
	}
	return s.persistFileLocked("users.json", s.users)
}

// GetIngameSN returns the base64-encoded ingamesn from the latest NAS login for profileID.
func (s *Store) GetIngameSN(profileID int64) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var userid string
	for _, u := range s.users {
		if u.ProfileID == profileID {
			userid = u.UserID
			break
		}
	}
	if userid == "" {
		return "", nil
	}

	for _, login := range s.nasLogins {
		if login.UserID != userid {
			continue
		}
		if v, ok := login.Data["ingamesn"]; ok {
			return v, nil
		}
	}
	return "", nil
}

// LoginProfileFromAuth creates or logs in a user from parsed NAS auth data.
func (s *Store) LoginProfileFromAuth(data map[string]string) (userid string, profileid int64, gsbrcd, uniquenick string, err error) {
	if data == nil {
		return "", 0, "", "", fmt.Errorf("json: missing auth data")
	}

	userid, ok := data["userid"]
	if !ok || userid == "" {
		return "", 0, "", "", fmt.Errorf("json: auth data missing userid")
	}

	gsbrcd = data["gsbrcd"]
	if gsbrcd == "" {
		return "", 0, "", "", fmt.Errorf("json: auth data missing gsbrcd")
	}

	console := 0
	if _, ok := data["passwd"]; !ok {
		console = 1
	}
	if _, ok := data["csnum"]; ok {
		console = 1
	}
	if _, ok := data["cfc"]; ok {
		console = 1
	}

	password := gsbrcd
	gameid := gsbrcd
	if len(gsbrcd) >= 4 {
		gameid = gsbrcd[:4]
	}

	userNum, err := strconv.ParseInt(userid, 10, 64)
	if err != nil {
		return "", 0, "", "", fmt.Errorf("json: invalid userid %q: %w", userid, err)
	}

	uniquenick = database.Base32Encode(userNum) + gsbrcd
	email := uniquenick + "@nds"

	csnum := data["csnum"]
	cfc := data["cfc"]
	bssid := data["bssid"]
	devname := data["devname"]
	birth := data["birth"]

	s.mu.Lock()
	defer s.mu.Unlock()

	idx := s.findUserLocked(userid, gsbrcd)
	if idx < 0 {
		profileid, err = s.createUserLocked(userid, password, email, uniquenick, gsbrcd, console, csnum, cfc, bssid, devname, birth, gameid)
		if err != nil {
			return "", 0, "", "", err
		}
		if profileid == 0 {
			return "", 0, "", "", fmt.Errorf("json: failed to create user")
		}
		storeLog.Debugf("login new user userid=%s profileid=%d gsbrcd=%s", userid, profileid, gsbrcd)
		return userid, profileid, gsbrcd, uniquenick, nil
	}

	u := s.users[idx]
	if u.Enabled != 1 {
		return "", 0, "", "", nil
	}
	storeLog.Debugf("login existing user userid=%s profileid=%d gsbrcd=%s", userid, u.ProfileID, gsbrcd)
	return userid, u.ProfileID, gsbrcd, uniquenick, nil
}

// CreateSession removes prior sessions for profileid and creates a new one.
func (s *Store) CreateSession(profileid int64) (sesskey, loginTicket string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.profileExistsLocked(profileid) {
		return "", "", fmt.Errorf("json: profile %d does not exist", profileid)
	}

	filtered := make([]sessionRecord, 0, len(s.sessions))
	for _, sess := range s.sessions {
		if sess.ProfileID != profileid {
			filtered = append(filtered, sess)
		}
	}
	s.sessions = filtered

	sesskey, err = s.generateSessionKeyLocked(8)
	if err != nil {
		return "", "", err
	}

	ticketBytes := make([]byte, 16)
	if _, err := rand.Read(ticketBytes); err != nil {
		return "", "", fmt.Errorf("json: generate login ticket: %w", err)
	}
	loginTicket = database.GamespyBase64Encode(ticketBytes)

	s.sessions = append(s.sessions, sessionRecord{
		Session:     sesskey,
		ProfileID:   profileid,
		LoginTicket: loginTicket,
	})
	if err := s.persistFileLocked("sessions.json", s.sessions); err != nil {
		return "", "", err
	}
	storeLog.Debugf("session created profileid=%d", profileid)
	return sesskey, loginTicket, nil
}

// DeleteSession removes a session by session key.
func (s *Store) DeleteSession(sesskey string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filtered := make([]sessionRecord, 0, len(s.sessions))
	deleted := false
	for _, sess := range s.sessions {
		if sess.Session != sesskey {
			filtered = append(filtered, sess)
		} else {
			deleted = true
		}
	}
	s.sessions = filtered
	if deleted {
		storeLog.Debugf("session deleted")
	}
	return s.persistFileLocked("sessions.json", s.sessions)
}

func (s *Store) findUserLocked(userid, gsbrcd string) int {
	for i, u := range s.users {
		if u.UserID == userid && u.Gsbrcd == gsbrcd {
			return i
		}
	}
	return -1
}

func (s *Store) profileExistsLocked(profileid int64) bool {
	for _, u := range s.users {
		if u.ProfileID == profileid {
			return true
		}
	}
	return false
}

func (s *Store) nextProfileIDLocked() int64 {
	var maxID int64
	for _, u := range s.users {
		if u.ProfileID > maxID {
			maxID = u.ProfileID
		}
	}
	return maxID + 1
}

func (s *Store) createUserLocked(userid, password, email, uniquenick, gsbrcd string, console int, csnum, cfc, bssid, devname, birth, gameid string) (int64, error) {
	if s.findUserLocked(userid, gsbrcd) >= 0 {
		return 0, nil
	}

	profileid := s.nextProfileIDLocked()
	hash := md5.Sum([]byte(password))
	hashedPassword := fmt.Sprintf("%x", hash)

	s.users = append(s.users, userRecord{
		ProfileID:  profileid,
		UserID:     userid,
		Password:   hashedPassword,
		Gsbrcd:     gsbrcd,
		Email:      email,
		Uniquenick: uniquenick,
		PID:        "11",
		Lon:        "0.000000",
		Lat:        "0.000000",
		Console:    console,
		Csnum:      csnum,
		Cfc:        cfc,
		Bssid:      bssid,
		Devname:    devname,
		Birth:      birth,
		GameID:     gameid,
		Enabled:    1,
	})
	if err := s.persistFileLocked("users.json", s.users); err != nil {
		return 0, err
	}
	return profileid, nil
}

func (s *Store) generateSessionKeyLocked(size int) (string, error) {
	for {
		key, err := database.RandomDecimalString(size)
		if err != nil {
			return "", err
		}
		exists := false
		for _, sess := range s.sessions {
			if sess.Session == key {
				exists = true
				break
			}
		}
		if !exists {
			return key, nil
		}
	}
}

func (s *Store) loadAll() error {
	if err := loadJSON(filepath.Join(s.dir, "users.json"), &s.users); err != nil {
		return err
	}
	if err := loadJSON(filepath.Join(s.dir, "sessions.json"), &s.sessions); err != nil {
		return err
	}
	if err := loadJSON(filepath.Join(s.dir, "nas_logins.json"), &s.nasLogins); err != nil {
		return err
	}
	if err := loadJSON(filepath.Join(s.dir, "banned.json"), &s.banned); err != nil {
		return err
	}
	if s.users == nil {
		s.users = []userRecord{}
	}
	if s.sessions == nil {
		s.sessions = []sessionRecord{}
	}
	if s.nasLogins == nil {
		s.nasLogins = []nasLoginRecord{}
	}
	if s.banned == nil {
		s.banned = []banRecord{}
	}
	return nil
}

func (s *Store) persistAllLocked() error {
	if err := s.persistFileLocked("users.json", s.users); err != nil {
		return err
	}
	if err := s.persistFileLocked("sessions.json", s.sessions); err != nil {
		return err
	}
	if err := s.persistFileLocked("nas_logins.json", s.nasLogins); err != nil {
		return err
	}
	return s.persistFileLocked("banned.json", s.banned)
}

func (s *Store) persistFileLocked(name string, v any) error {
	path := filepath.Join(s.dir, name)
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		storeLog.Errorf("persist marshal failed file=%s err=%v", name, err)
		return fmt.Errorf("json: marshal %s: %w", name, err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		storeLog.Errorf("persist write failed file=%s err=%v", name, err)
		return fmt.Errorf("json: write %s: %w", name, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		storeLog.Errorf("persist rename failed file=%s err=%v", name, err)
		return fmt.Errorf("json: rename %s: %w", name, err)
	}
	return nil
}

func loadJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("json: read %s: %w", path, err)
	}
	if len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("json: decode %s: %w", path, err)
	}
	return nil
}
