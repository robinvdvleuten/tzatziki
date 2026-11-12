package tzatziki

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/emersion/go-ical"
)

// Definitions come from strangers, so no input may panic or take long, and a
// definition that reads must give a zone that works.
func FuzzToLocation(f *testing.F) {
	paths, err := filepath.Glob(filepath.Join("testdata", "*.ics"))
	if err != nil || len(paths) == 0 {
		f.Fatalf("no seed fixtures found: %v", err)
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(data)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		cal, err := ical.NewDecoder(bytes.NewReader(data)).Decode()
		if err != nil {
			return
		}
		for _, child := range cal.Children {
			if child.Name != ical.CompTimezone {
				continue
			}
			began := time.Now()
			loc, err := ToLocation(child)
			if took := time.Since(began); took > 5*time.Second {
				t.Fatalf("ToLocation took %v", took)
			}
			if err != nil {
				continue
			}
			at := time.Date(2026, 6, 1, 0, 0, 0, 0, loc)
			if _, offset := at.Zone(); offset <= -100*3600 || offset >= 100*3600 {
				t.Fatalf("offset %d is out of range", offset)
			}
		}
	})
}

// Whatever zone Go has, its VTIMEZONE reads back to the same offsets.
func FuzzFromLocation(f *testing.F) {
	f.Add("Europe/Amsterdam", 2026, uint8(10))
	f.Add("Australia/Lord_Howe", 2085, uint8(3))
	f.Add("America/St_Johns", 1950, uint8(1))

	f.Fuzz(func(t *testing.T, name string, year int, years uint8) {
		loc := namedLocation(name)
		if loc == nil || year < 1900 || year > 2090 || years == 0 || years > 20 {
			return
		}
		from := time.Date(year, 1, 1, 0, 0, 0, 0, loc)
		until := from.AddDate(int(years), 0, 0)
		component, err := FromLocation(loc, from, until)
		if err != nil {
			t.Fatal(err)
		}
		read, err := ToLocation(component)
		if err != nil {
			t.Fatalf("read back: %v", err)
		}
		for at := from.Add(24 * time.Hour); at.Before(until.Add(-24 * time.Hour)); at = at.Add(time.Hour) {
			_, want := at.Zone()
			_, got := at.In(read).Zone()
			if got != want {
				t.Fatalf("at %s the offset is %d, Go says %d", at.UTC().Format(time.RFC3339), got, want)
			}
		}
	})
}
