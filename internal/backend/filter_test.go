
package backend

import "testing"

const mkwiiFilter = "dwc_mver = 90 and dwc_pid != 1 and maxplayers = 11 and numplayers < 11 and dwc_mtype = 0 and dwc_hoststate = 2 and dwc_suspend = 0 and (rk = 'vs_123' and (ev > 4263 or ev <= 5763) and p = 0)"

func mkwiiServer(ev string) map[string]string {
	return map[string]string{
		"dwc_mver":      "90",
		"dwc_pid":       "2",
		"maxplayers":    "11",
		"numplayers":    "10",
		"dwc_mtype":     "0",
		"dwc_hoststate": "2",
		"dwc_suspend":   "0",
		"rk":            "vs_123",
		"ev":            ev,
		"p":             "0",
	}
}

func TestMatchFilterMKWii(t *testing.T) {
	ok, err := MatchFilter(mkwiiServer("5000"), mkwiiFilter)
	if err != nil {
		t.Fatalf("MatchFilter: %v", err)
	}
	if !ok {
		t.Fatal("expected matching MKWii server to pass filter")
	}

	ok, err = MatchFilter(mkwiiServer("4000"), mkwiiFilter)
	if err != nil {
		t.Fatalf("MatchFilter ev=4000: %v", err)
	}
	if !ok {
		t.Fatal("expected ev=4000 to pass via ev <= 5763")
	}

	ok, err = MatchFilter(mkwiiServer("6000"), mkwiiFilter)
	if err != nil {
		t.Fatalf("MatchFilter ev=6000: %v", err)
	}
	if !ok {
		t.Fatal("expected ev=6000 to pass via ev > 4263")
	}

	failCases := []struct {
		name   string
		server map[string]string
	}{
		{"dwc_pid=1", func() map[string]string { s := mkwiiServer("5000"); s["dwc_pid"] = "1"; return s }()},
		{"dwc_mver wrong", func() map[string]string { s := mkwiiServer("5000"); s["dwc_mver"] = "89"; return s }()},
		{"numplayers full", func() map[string]string { s := mkwiiServer("5000"); s["numplayers"] = "11"; return s }()},
		{"rk mismatch", func() map[string]string { s := mkwiiServer("5000"); s["rk"] = "vs_999"; return s }()},
		{"p wrong", func() map[string]string { s := mkwiiServer("5000"); s["p"] = "1"; return s }()},
	}
	for _, tc := range failCases {
		t.Run(tc.name, func(t *testing.T) {
			ok, err := MatchFilter(tc.server, mkwiiFilter)
			if err != nil {
				t.Fatalf("MatchFilter: %v", err)
			}
			if ok {
				t.Fatalf("expected %s to fail filter", tc.name)
			}
		})
	}
}

func TestMatchFilterCrossFieldAndBitwise(t *testing.T) {
	server := map[string]string{
		"dwc_mresv": "474890913",
		"dwc_pid":   "100",
		"mskdif":    "1",
		"mskstg":    "14",
		"auth":      "20",
	}
	ok, err := MatchFilter(server, "dwc_mresv != dwc_pid")
	if err != nil || !ok {
		t.Fatalf("cross-field != : ok=%v err=%v", ok, err)
	}

	ok, err = MatchFilter(server, "((1&mskdif)=mskdif) and ((14&mskstg)=mskstg) and (20=auth)")
	if err != nil || !ok {
		t.Fatalf("bitwise filter: ok=%v err=%v", ok, err)
	}

	ok, err = MatchFilter(map[string]string{"zvar": "102"}, "zvar LIKE '102'")
	if err != nil || !ok {
		t.Fatalf("LIKE: ok=%v err=%v", ok, err)
	}
	ok, err = MatchFilter(map[string]string{"zvar": "ABC"}, "zvar LIKE 'abc'")
	if err != nil || !ok {
		t.Fatalf("LIKE case-insensitive: ok=%v err=%v", ok, err)
	}
}

func TestBackendFindServers(t *testing.T) {
	b := New()
	_ = b.UpdateServerList("mariokartwii", 1, mkwiiServer("5000"), 1)
	_ = b.UpdateServerList("mariokartwii", 2, func() map[string]string {
		s := mkwiiServer("5000")
		s["dwc_pid"] = "1"
		return s
	}(), 1)

	results, err := b.FindServers("mariokartwii", mkwiiFilter, []string{"dwc_pid", "ev"}, 10)
	if err != nil {
		t.Fatalf("FindServers: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Requested["dwc_pid"] != "2" {
		t.Fatalf("expected dwc_pid=2, got %q", results[0].Requested["dwc_pid"])
	}
}

func TestDeleteServerBySessionID(t *testing.T) {
	b := New()
	fields := mkwiiServer("5000")
	fields["dwc_pid"] = "1"
	_ = b.UpdateServerList("mariokartwii", 0xabcdef01, fields, 1)

	results, err := b.FindServers("mariokartwii", "dwc_pid = 1", []string{"dwc_pid"}, 0)
	if err != nil || len(results) != 1 {
		t.Fatalf("before delete: err=%v len=%d", err, len(results))
	}

	b.DeleteServer("mariokartwii", 0xabcdef01)

	results, err = b.FindServers("mariokartwii", "dwc_pid = 1", []string{"dwc_pid"}, 0)
	if err != nil {
		t.Fatalf("after delete: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("after delete: expected 0 rooms, got %d", len(results))
	}
}
