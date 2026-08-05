package gpsp

import (
	"strings"
	"testing"

	"github.com/IrishBruse/mkw-dwc/internal/database"
)

type fakeStore struct {
	profiles map[int64]*database.Profile
}

func (f *fakeStore) Close() error { return nil }
func (f *fakeStore) Initialize() error { return nil }
func (f *fakeStore) GetNextAvailableUserid() string { return "" }
func (f *fakeStore) IsBanned(_, _ string) bool { return false }
func (f *fakeStore) StoreNasLogin(_, _ string, _ map[string]string) error { return nil }
func (f *fakeStore) GetNasLogin(_ string) (map[string]string, error) { return nil, nil }
func (f *fakeStore) GetIngameSN(_ int64) (string, error) { return "", nil }
func (f *fakeStore) GetProfile(profileID int64) (*database.Profile, error) {
	if p, ok := f.profiles[profileID]; ok {
		return p, nil
	}
	return nil, nil
}
func (f *fakeStore) UpdateProfile(_ int64, _ map[string]string) error { return nil }
func (f *fakeStore) LoginProfileFromAuth(_ map[string]string) (string, int64, string, string, error) {
	return "", 0, "", "", nil
}
func (f *fakeStore) CreateSession(_ int64) (string, string, error) { return "", "", nil }
func (f *fakeStore) DeleteSession(_ string) error { return nil }

func TestBuildOtherslistReply(t *testing.T) {
	db := &fakeStore{
		profiles: map[int64]*database.Profile{
			123: {ProfileID: 123, Uniquenick: "alice"},
			456: {ProfileID: 456, Uniquenick: "bob"},
		},
	}

	got := string(buildOtherslistReply("123|999|456", db))
	want := `\otherslist\\o\123\uniquenick\alice\o\999\uniquenick\\o\456\uniquenick\bob\oldone\\final\`
	if got != want {
		t.Fatalf("buildOtherslistReply mismatch:\ngot  %q\nwant %q", got, want)
	}

	empty := string(buildOtherslistReply("", db))
	if empty != `\otherslist\\oldone\\final\` {
		t.Fatalf("empty opids reply: got %q", empty)
	}

	if !strings.HasSuffix(got, `\final\`) {
		t.Fatalf("reply does not end with \\final\\: %q", got)
	}
}
