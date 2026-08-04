// Package database defines the GPCM profile and NAS login persistence interface.
package database

// Store is durable storage for users, sessions, NAS logins, and bans.
type Store interface {
	Close() error
	Initialize() error
	GetNextAvailableUserid() string
	IsBanned(gamecd, ipaddr string) bool
	StoreNasLogin(userid, authtoken string, data map[string]string) error
	GetNasLogin(authtoken string) (map[string]string, error)
	GetIngameSN(profileID int64) (string, error)
	LoginProfileFromAuth(data map[string]string) (userid string, profileid int64, gsbrcd, uniquenick string, err error)
	CreateSession(profileid int64) (sesskey, loginTicket string, err error)
	DeleteSession(sesskey string) error
}
