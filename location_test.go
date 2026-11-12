package tzatziki

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-ical"
)

func readCalendar(t testing.TB, data []byte) *ical.Calendar {
	t.Helper()
	cal, err := ical.NewDecoder(bytes.NewReader(data)).Decode()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return cal
}

func fixture(t *testing.T, name string) *ical.Calendar {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name+".ics"))
	if err != nil {
		t.Fatal(err)
	}
	return readCalendar(t, data)
}

func vtimezone(t *testing.T, lines ...string) *ical.Component {
	t.Helper()
	text := strings.Join(append(append([]string{"BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//tzatziki//test//EN"}, lines...), "END:VCALENDAR", ""), "\r\n")
	cal := readCalendar(t, []byte(text))
	if len(cal.Children) != 1 {
		t.Fatalf("want one component, got %d", len(cal.Children))
	}
	return cal.Children[0]
}

// offsetAt returns loc's offset in hours at the given UTC time.
func offsetAt(loc *time.Location, utc string) float64 {
	at, err := time.Parse(time.RFC3339, utc)
	if err != nil {
		panic(err)
	}
	_, offset := at.In(loc).Zone()
	return float64(offset) / 3600
}

func TestToLocationOutlookRules(t *testing.T) {
	// Outlook starts its yearly rules in 1601, past what rrule-go follows.
	cal := fixture(t, "vtimezone-outlook")

	loc, err := ToLocation(cal.Children[0])
	if err != nil {
		t.Fatal(err)
	}

	tests := map[string]float64{
		"2026-03-29T00:59:59Z": 1,
		"2026-03-29T01:00:00Z": 2,
		"2026-10-25T00:59:59Z": 2,
		"2026-10-25T01:00:00Z": 1,
		"1950-07-01T00:00:00Z": 2,
		"2099-07-01T00:00:00Z": 2,
	}
	for at, want := range tests {
		if got := offsetAt(loc, at); got != want {
			t.Errorf("at %s offset = %v, want %v", at, got, want)
		}
	}
	if loc.String() != "" {
		t.Errorf("name = %q, want none", loc.String())
	}
}

func TestToLocationFixedOffset(t *testing.T) {
	loc, err := ToLocation(fixture(t, "vtimezone-fixed").Children[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, at := range []string{"1960-01-01T00:00:00Z", "2026-11-10T00:00:00Z"} {
		if got := offsetAt(loc, at); got != 5.5 {
			t.Errorf("at %s offset = %v", at, got)
		}
	}
}

func TestToLocationRDATEAndUNTIL(t *testing.T) {
	component := vtimezone(t,
		"BEGIN:VTIMEZONE", "TZID:Test/Rules",
		"BEGIN:STANDARD", "DTSTART:20001029T030000", "TZOFFSETFROM:+0200", "TZOFFSETTO:+0100",
		"RRULE:FREQ=YEARLY;BYDAY=-1SU;BYMONTH=10;UNTIL=20021027T010000Z", "END:STANDARD",
		"BEGIN:DAYLIGHT", "DTSTART:20000326T020000", "TZOFFSETFROM:+0100", "TZOFFSETTO:+0200",
		"RDATE:20010325T020000,20020331T020000", "END:DAYLIGHT",
		"END:VTIMEZONE")

	loc, err := ToLocation(component)
	if err != nil {
		t.Fatal(err)
	}

	tests := map[string]float64{
		"2002-10-27T00:59:59Z": 2,
		"2002-10-27T01:00:00Z": 1, // UNTIL is the UTC time of the last change.
		"2002-07-01T00:00:00Z": 2,
		"2003-07-01T00:00:00Z": 1, // No RDATE for 2003.
	}
	for at, want := range tests {
		if got := offsetAt(loc, at); got != want {
			t.Errorf("at %s offset = %v, want %v", at, got, want)
		}
	}
}

func TestToLocationRefusesBadDefinitions(t *testing.T) {
	standard := func(lines ...string) *ical.Component {
		return vtimezone(t, append(append([]string{"BEGIN:VTIMEZONE", "TZID:Broken", "BEGIN:STANDARD"}, lines...), "END:STANDARD", "END:VTIMEZONE")...)
	}
	tests := []struct {
		name      string
		component *ical.Component
		want      string
	}{
		{"nil", nil, "not a VTIMEZONE"},
		{"another component", ical.NewComponent(ical.CompEvent), "not a VTIMEZONE"},
		{"no observances", vtimezone(t, "BEGIN:VTIMEZONE", "TZID:Empty", "END:VTIMEZONE"), "no observances"},
		{"no offsets", standard("DTSTART:19700101T000000"), "TZOFFSETFROM"},
		{"bad offset", standard("DTSTART:19700101T000000", "TZOFFSETFROM:+0100", "TZOFFSETTO:soon"), "bad offset"},
		{"no start", standard("TZOFFSETFROM:+0100", "TZOFFSETTO:+0100"), "DTSTART"},
		{"bad start", standard("DTSTART:yesterday", "TZOFFSETFROM:+0100", "TZOFFSETTO:+0100"), "DTSTART"},
		{"bad RDATE", standard("DTSTART:19700101T000000", "TZOFFSETFROM:+0100", "TZOFFSETTO:+0100", "RDATE:soon"), "RDATE"},
		{"bad RRULE", standard("DTSTART:19700101T000000", "TZOFFSETFROM:+0100", "TZOFFSETTO:+0100", "RRULE:FREQ=SOMETIMES"), "RRULE"},
		// rrule-go panics while it starts this rule: there is no 20th Saturday in October.
		{"RRULE that can't start", standard("DTSTART:19700101T000000", "TZOFFSETFROM:+0100", "TZOFFSETTO:+0100", "RRULE:FREQ=YEARLY;BYDAY=20SA;BYMONTH=10"), "RRULE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ToLocation(tt.component)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}

func TestToLocationRefusesRuleThatRepeatsTooOften(t *testing.T) {
	component := vtimezone(t,
		"BEGIN:VTIMEZONE", "TZID:Hostile", "BEGIN:STANDARD", "DTSTART:19700101T000000",
		"TZOFFSETFROM:+0100", "TZOFFSETTO:+0200", "RRULE:FREQ=SECONDLY", "END:STANDARD", "END:VTIMEZONE")

	started := time.Now()
	_, err := ToLocation(component)

	if !errors.Is(err, ErrRuleTooExpensive) || !strings.Contains(err.Error(), "Hostile") {
		t.Errorf("err = %v, want ErrRuleTooExpensive naming the zone", err)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Errorf("took %s", elapsed)
	}
}
