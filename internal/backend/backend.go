package backend

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/IrishBruse/mkw-dwc/internal/gamespy"
	"github.com/IrishBruse/mkw-dwc/internal/logging"
)

const maxNatnegEntries = 8

// ServerRecord mirrors a QR heartbeat entry in the backend server list.
type ServerRecord struct {
	SessionID  uint32
	Console    int // 0=DS, 1=Wii
	Gamename   string
	PublicIP   string
	PublicPort string
	LocalPort  string
	LocalIP0   string
	Natneg     string
	Fields     map[string]string
}

// AsMap returns all server fields as a flat string map, including reserved keys.
func (r ServerRecord) AsMap() map[string]string {
	m := make(map[string]string, len(r.Fields)+12)
	for k, v := range r.Fields {
		m[k] = v
	}
	if r.PublicIP != "" {
		m["publicip"] = r.PublicIP
	}
	if r.PublicPort != "" {
		m["publicport"] = r.PublicPort
	}
	if r.LocalPort != "" {
		m["localport"] = r.LocalPort
	}
	if r.LocalIP0 != "" {
		m["localip0"] = r.LocalIP0
	}
	if r.Natneg != "" {
		m["natneg"] = r.Natneg
	}
	m["__session__"] = strconv.FormatUint(uint64(r.SessionID), 10)
	m["__console__"] = strconv.Itoa(r.Console)
	return m
}

// ServerResult is one match from FindServers with requested field subset.
type ServerResult struct {
	Record    ServerRecord
	Requested map[string]string
}

// LocalAddr is a host local endpoint from NATNEG INIT.
type LocalAddr struct {
	IP   [4]byte
	Port uint16
}

// Backend is an in-process server list and NATNEG registry.
type Backend struct {
	mu         sync.RWMutex
	serverList map[string][]ServerRecord
	sessionIdx map[string]map[uint32]int
	natnegList map[uint32][]map[string]string
}

// New returns an empty backend.
func New() *Backend {
	return &Backend{
		serverList: make(map[string][]ServerRecord),
		sessionIdx: make(map[string]map[uint32]int),
		natnegList: make(map[uint32][]map[string]string),
	}
}

// UpdateServerList merges heartbeat fields into the server list for gameid.
func (b *Backend) UpdateServerList(gameid string, session uint32, fields map[string]string, console int) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	rec := ServerRecord{
		SessionID: session,
		Console:   console,
		Gamename:  gameid,
		Fields:    make(map[string]string, len(fields)),
	}
	for k, v := range fields {
		switch k {
		case "publicip":
			rec.PublicIP = v
		case "publicport":
			rec.PublicPort = v
		case "localport":
			rec.LocalPort = v
		case "localip0":
			rec.LocalIP0 = v
		case "natneg":
			rec.Natneg = v
		default:
			rec.Fields[k] = v
		}
	}

	idxMap := b.sessionIdx[gameid]
	if idxMap == nil {
		idxMap = make(map[uint32]int)
		b.sessionIdx[gameid] = idxMap
	}
	if idx, ok := idxMap[session]; ok {
		b.serverList[gameid][idx] = rec
		logging.For("backend").Debugf("room upsert gamename=%s session=%08x update=true", gameid, session)
	} else {
		b.serverList[gameid] = append(b.serverList[gameid], rec)
		idxMap[session] = len(b.serverList[gameid]) - 1
		logging.For("backend").Debugf("room upsert gamename=%s session=%08x update=false", gameid, session)
	}
	return nil
}

// DeleteServer removes all servers for gameid with the given session id.
func (b *Backend) DeleteServer(gameid string, session uint32) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.deleteServerLocked(gameid, session)
}

func (b *Backend) deleteServerLocked(gameid string, session uint32) {
	idxMap := b.sessionIdx[gameid]
	if idxMap == nil {
		return
	}
	idx, ok := idxMap[session]
	if !ok {
		return
	}

	logging.For("backend").Debugf("room deleted gamename=%s session=%08x", gameid, session)

	servers := b.serverList[gameid]
	servers = append(servers[:idx], servers[idx+1:]...)
	delete(idxMap, session)

	for i, s := range servers {
		idxMap[s.SessionID] = i
	}

	if len(servers) == 0 {
		delete(b.serverList, gameid)
		delete(b.sessionIdx, gameid)
		return
	}
	b.serverList[gameid] = servers
}

// FindServers returns servers matching filter with the requested field subset.
func (b *Backend) FindServers(gameid, filter string, fields []string, maxCount int) ([]ServerResult, error) {
	start := time.Now()
	defer logging.LogDuration("backend", "FindServers", start)

	compiled, err := CompileFilter(filter)
	if err != nil {
		return nil, err
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	servers, ok := b.serverList[gameid]
	if !ok {
		return nil, nil
	}

	var matched []ServerResult
	for _, rec := range servers {
		ok, err := compiled.Match(rec.AsMap())
		if err != nil || !ok {
			continue
		}

		matched = append(matched, buildServerResult(rec, fields))
		if maxCount > 0 && len(matched) >= maxCount {
			break
		}
	}
	logging.For("backend").Debugf("find servers gamename=%s filter_len=%d rooms=%d matched=%d", gameid, len(filter), len(servers), len(matched))
	return matched, nil
}

// FindServerByAddress looks up a server by public IP and optional port.
func (b *Backend) FindServerByAddress(ip string, port int, gameid string) *ServerRecord {
	b.mu.RLock()
	defer b.mu.RUnlock()

	portStr := ""
	if port != 0 {
		portStr = strconv.Itoa(port)
	}

	search := func(servers []ServerRecord) *ServerRecord {
		for i := range servers {
			s := &servers[i]
			if !gamespy.MatchPublicIP(s.PublicIP, ip) {
				continue
			}
			if portStr == "" || s.PublicPort == portStr {
				return s
			}
		}
		return nil
	}

	if gameid == "" {
		for _, servers := range b.serverList {
			if rec := search(servers); rec != nil {
				return rec
			}
		}
		return nil
	}

	return search(b.serverList[gameid])
}

// FindServerByLocalAddress looks up a server by public IP and local endpoint.
func (b *Backend) FindServerByLocalAddress(publicIP string, local LocalAddr, gameid string) *ServerRecord {
	b.mu.RLock()
	defer b.mu.RUnlock()

	localIP := formatIP(local.IP)
	localPort := strconv.Itoa(int(local.Port))

	findInGame := func(gid string) *ServerRecord {
		servers, ok := b.serverList[gid]
		if !ok {
			return nil
		}

		var bestMatch *ServerRecord
		for i := range servers {
			s := &servers[i]
			if !gamespy.MatchPublicIP(s.PublicIP, publicIP) {
				continue
			}
			if s.LocalPort == localPort {
				return s
			}
			for x := 0; x < 10; x++ {
				key := fmt.Sprintf("localip%d", x)
				if v, ok := s.Fields[key]; ok && v == localIP {
					bestMatch = s
				}
				if x == 0 && s.LocalIP0 == localIP {
					bestMatch = s
				}
			}
			if local.Port == 0 && bestMatch == nil {
				bestMatch = s
			}
		}
		return bestMatch
	}

	if gameid == "" {
		for gid := range b.serverList {
			if rec := findInGame(gid); rec != nil {
				return rec
			}
		}
		return nil
	}

	return findInGame(gameid)
}

// AddNatnegServer registers a NATNEG server entry under cookie.
func (b *Backend) AddNatnegServer(cookie uint32, server map[string]string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	publicIP := server["publicip"]
	publicPort := server["publicport"]
	for _, existing := range b.natnegList[cookie] {
		if existing["publicip"] == publicIP && existing["publicport"] == publicPort {
			return
		}
	}

	entries := b.natnegList[cookie]
	if len(entries) >= maxNatnegEntries {
		logging.For("backend").Warnf("natneg entry cap hit cookie=%08x", cookie)
		return
	}
	b.natnegList[cookie] = append(entries, server)
	logging.For("backend").Debugf("natneg add cookie=%08x publicip=%s publicport=%s", cookie, publicIP, publicPort)
}

// GetNatnegServer returns NATNEG entries for cookie, or nil if none exist.
func (b *Backend) GetNatnegServer(cookie uint32) []map[string]string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	entries := b.natnegList[cookie]
	if len(entries) == 0 {
		return nil
	}
	out := make([]map[string]string, len(entries))
	for i, e := range entries {
		cp := make(map[string]string, len(e))
		for k, v := range e {
			cp[k] = v
		}
		out[i] = cp
	}
	return out
}

// DeleteNatnegServer removes all NATNEG entries for cookie.
func (b *Backend) DeleteNatnegServer(cookie uint32) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.natnegList[cookie]; ok {
		delete(b.natnegList, cookie)
		logging.For("backend").Debugf("natneg deleted cookie=%08x", cookie)
	}
}

func buildServerResult(rec ServerRecord, fields []string) ServerResult {
	fm := rec.AsMap()
	requested := make(map[string]string, len(fields))
	for _, field := range fields {
		if v, ok := fm[field]; ok {
			requested[field] = v
		} else {
			requested[field] = ""
		}
	}
	return ServerResult{
		Record:    rec,
		Requested: requested,
	}
}

func formatIP(ip [4]byte) string {
	return fmt.Sprintf("%d.%d.%d.%d", ip[0], ip[1], ip[2], ip[3])
}
