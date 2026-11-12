package tzatziki

import (
	"fmt"
	"testing"
	"time"

	"github.com/emersion/go-ical"
)

// horizonYears is how many years after the first the tests cover, as pim-mcp
// writes them.
const horizonYears = 10

func loadZone(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

// fromYear calls FromLocation as pim-mcp does: from 1 January of year through
// the horizon.
func fromYear(loc *time.Location, year int) (*ical.Component, error) {
	return FromLocation(loc, time.Date(year, time.January, 1, 0, 0, 0, 0, loc), time.Date(year+horizonYears+1, time.January, 1, 0, 0, 0, 0, loc))
}

func TestFromLocationMatchesGo(t *testing.T) {
	zones := []string{"Europe/Amsterdam", "America/New_York", "Australia/Sydney", "Asia/Kolkata", "Africa/Casablanca", "Australia/Lord_Howe", "America/St_Johns", "UTC"}
	for _, name := range zones {
		for _, year := range []int{2012, 2026, 2085} {
			t.Run(fmt.Sprintf("%s from %d", name, year), func(t *testing.T) {
				checkTimezoneMatchesGo(t, loadZone(t, name), year)
			})
		}
	}
}

// macOS ships zone data in the rearguard format, which has no negative DST. There
// zic writes an offset kept all year as DST all year, with a placeholder standard
// time that is never in force. Go puts that placeholder in force from 22:00 to
// midnight UTC on 31 December, and in a leap year it answers with a zone end before
// the time asked about. Casablanca from 2087 in tzdata before 2026f is like this.
func TestFromLocationWithDaylightTimeAllYear(t *testing.T) {
	data, err := encodeTZif(3600, nil)
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, "\nXXX-2<+01>-1,0/0,J365/23\n"...)
	loc, err := time.LoadLocationFromTZData("Test/AllYear", data)
	if err != nil {
		t.Fatal(err)
	}
	// 2088 is a leap year.
	checkTimezoneMatchesGo(t, loc, 2085)
}

// checkTimezoneMatchesGo builds the VTIMEZONE for loc from year, reads it back,
// and compares the offsets hour by hour with Go's.
func checkTimezoneMatchesGo(t *testing.T, loc *time.Location, year int) {
	t.Helper()
	component, err := fromYear(loc, year)
	if err != nil {
		t.Fatal(err)
	}

	read, err := ToLocation(component)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	from := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC).Add(-24 * time.Hour)
	until := time.Date(year+horizonYears+1, 1, 1, 0, 0, 0, 0, time.UTC).Add(24 * time.Hour)
	checked := 0
	for at := from; at.Before(until); at = at.Add(time.Hour) {
		// The definition starts at local midnight on 1 January, which is within
		// a day of the UTC instant used here.
		if at.Before(from.Add(24*time.Hour)) || !at.Before(until.Add(-24*time.Hour)) {
			continue
		}
		_, want := at.In(loc).Zone()
		_, got := at.In(read).Zone()
		if got != want {
			t.Fatalf("at %s the offset is %d, Go says %d", at.Format(time.RFC3339), got, want)
		}
		checked++
	}
	if checked < 24*365*horizonYears {
		t.Errorf("checked only %d hours", checked)
	}
}

func TestFromLocationShape(t *testing.T) {
	kolkata := loadZone(t, "Asia/Kolkata")
	component, err := fromYear(kolkata, 2026)
	if err != nil {
		t.Fatal(err)
	}
	if len(component.Children) != 1 || component.Children[0].Name != "STANDARD" {
		t.Fatalf("a zone without clock changes wants one STANDARD observance, got %+v", component.Children)
	}
	if got := component.Children[0].Props.Get("TZOFFSETTO").Value; got != "+0530" {
		t.Errorf("offset = %s", got)
	}

	amsterdam := loadZone(t, "Europe/Amsterdam")
	component, err = fromYear(amsterdam, 2026)
	if err != nil {
		t.Fatal(err)
	}
	if got := component.Props.Get("TZID").Value; got != "Europe/Amsterdam" {
		t.Errorf("TZID = %s", got)
	}
	// One to fix the start, then two a year for 2026 through 2036.
	if len(component.Children) != 1+2*(horizonYears+1) {
		t.Errorf("observances = %d", len(component.Children))
	}
	second := component.Children[1]
	for name, want := range map[string]string{"DTSTART": "20260329T020000", "TZOFFSETFROM": "+0100", "TZOFFSETTO": "+0200", "TZNAME": "CEST"} {
		if got := second.Props.Get(name).Value; got != want {
			t.Errorf("first clock change %s = %s, want %s", name, got, want)
		}
	}
}

func TestFromLocationOffsetWithSeconds(t *testing.T) {
	// Amsterdam kept its own time, 19 minutes 32 seconds ahead of UTC, until 1937.
	loc := time.FixedZone("AMT", 19*60+32)
	component, err := fromYear(loc, 1930)
	if err != nil {
		t.Fatal(err)
	}
	if got := component.Children[0].Props.Get("TZOFFSETTO").Value; got != "+001932" {
		t.Errorf("offset = %s", got)
	}
	read, err := ToLocation(component)
	if err != nil {
		t.Fatal(err)
	}
	if _, offset := time.Date(1930, 6, 1, 0, 0, 0, 0, read).Zone(); offset != 19*60+32 {
		t.Errorf("read back offset = %d", offset)
	}
}

func TestFromLocationRefusesBadArguments(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name        string
		loc         *time.Location
		from, until time.Time
	}{
		{"no location", nil, at, at.AddDate(1, 0, 0)},
		{"empty range", time.UTC, at, at},
		{"range backwards", time.UTC, at, at.AddDate(-1, 0, 0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := FromLocation(tt.loc, tt.from, tt.until); err == nil {
				t.Error("want an error")
			}
		})
	}
}
