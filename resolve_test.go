package tzatziki

import (
	"errors"
	"strings"
	"sync"
	"testing"
)

// tzid returns the TZID parameter of the first event's DTSTART.
func tzid(t *testing.T, name string) (*Resolver, string) {
	t.Helper()
	cal := fixture(t, name)
	for _, event := range cal.Events() {
		return NewResolver(cal), event.Props.Get("DTSTART").Params.Get("TZID")
	}
	t.Fatal("no event")
	return nil, ""
}

func TestResolverFixtures(t *testing.T) {
	tests := []struct {
		fixture string
		want    string // the zone's name, empty for a definition
		offset  float64
	}{
		{"windows-zone", "Europe/Berlin", 1},
		{"tzid-prefix", "Europe/Amsterdam", 1},
		{"vtimezone-outlook", "", 1},
		{"vtimezone-fixed", "", 5.5},
	}
	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			r, id := tzid(t, tt.fixture)

			loc, err := r.Location(id)
			if err != nil {
				t.Fatal(err)
			}
			if loc.String() != tt.want {
				t.Errorf("name = %q, want %q", loc.String(), tt.want)
			}
			if got := offsetAt(loc, "2026-11-10T09:00:00Z"); got != tt.offset {
				t.Errorf("offset = %v, want %v", got, tt.offset)
			}
		})
	}
}

func TestResolverPrefersNameOverDefinition(t *testing.T) {
	// The sender's definition says Amsterdam is always at +05:30.
	cal := readCalendar(t, []byte(strings.Join([]string{
		"BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//tzatziki//test//EN",
		"BEGIN:VTIMEZONE", "TZID:Europe/Amsterdam", "BEGIN:STANDARD", "DTSTART:19700101T000000",
		"TZOFFSETFROM:+0530", "TZOFFSETTO:+0530", "END:STANDARD", "END:VTIMEZONE",
		"END:VCALENDAR", "",
	}, "\r\n")))

	loc, err := NewResolver(cal).Location("Europe/Amsterdam")
	if err != nil {
		t.Fatal(err)
	}
	if loc.String() != "Europe/Amsterdam" || offsetAt(loc, "2026-11-10T09:00:00Z") != 1 {
		t.Errorf("got %s at %v", loc, offsetAt(loc, "2026-11-10T09:00:00Z"))
	}
}

func TestResolverRefusesUnknownZones(t *testing.T) {
	tests := []struct {
		name string
		tzid string
		want string
	}{
		{"unknown, no definition", "Mars/Olympus", "Mars/Olympus"},
		{"the machine's zone isn't a TZID", "Local", "Local"},
		{"empty", "", `""`},
		{"relative path", "../../etc/passwd", "etc/passwd"},
		{"prefix of nothing known", "/example.org/Mars/Olympus", "Mars/Olympus"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewResolver(nil).Location(tt.tzid)

			if !errors.Is(err, ErrUnknownZone) || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want ErrUnknownZone mentioning %q", err, tt.want)
			}
			if _, err := LoadLocation(tt.tzid); !errors.Is(err, ErrUnknownZone) {
				t.Errorf("LoadLocation err = %v, want ErrUnknownZone", err)
			}
		})
	}
}

func TestResolverKeepsErrors(t *testing.T) {
	cal := readCalendar(t, []byte(strings.Join([]string{
		"BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//tzatziki//test//EN",
		"BEGIN:VTIMEZONE", "TZID:Hostile", "BEGIN:STANDARD", "DTSTART:19700101T000000",
		"TZOFFSETFROM:+0100", "TZOFFSETTO:+0200", "RRULE:FREQ=SECONDLY", "END:STANDARD", "END:VTIMEZONE",
		"END:VCALENDAR", "",
	}, "\r\n")))
	r := NewResolver(cal)

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, err := r.Location("Hostile"); !errors.Is(err, ErrRuleTooExpensive) {
				t.Errorf("err = %v, want ErrRuleTooExpensive", err)
			}
		})
	}
	wg.Wait()
	if len(r.resolved) != 1 {
		t.Errorf("resolved = %v", r.resolved)
	}
}

func TestWindowsToIANA(t *testing.T) {
	if got, ok := WindowsToIANA("W. Europe Standard Time"); !ok || got != "Europe/Berlin" {
		t.Errorf("got %q, %v", got, ok)
	}
	if got, ok := WindowsToIANA("Europe/Berlin"); ok {
		t.Errorf("an IANA name is no Windows name, got %q", got)
	}
}
