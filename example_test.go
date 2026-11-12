package tzatziki_test

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/emersion/go-ical"
	"github.com/robinvdvleuten/tzatziki"
)

func ExampleFromLocation() {
	amsterdam, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		log.Fatal(err)
	}
	from := time.Date(2026, time.January, 1, 0, 0, 0, 0, amsterdam)

	vtimezone, err := tzatziki.FromLocation(amsterdam, from, from.AddDate(1, 0, 0))
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(vtimezone.Props.Get(ical.PropTimezoneID).Value)
	for _, observance := range vtimezone.Children {
		fmt.Println(observance.Name,
			observance.Props.Get(ical.PropDateTimeStart).Value,
			observance.Props.Get(ical.PropTimezoneOffsetTo).Value)
	}
	// Output:
	// Europe/Amsterdam
	// STANDARD 20260101T000000 +0100
	// DAYLIGHT 20260329T020000 +0200
	// STANDARD 20261025T030000 +0100
}

func ExampleToLocation() {
	cal, err := ical.NewDecoder(strings.NewReader(`BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//example//EN
BEGIN:VTIMEZONE
TZID:Central Europe
BEGIN:STANDARD
DTSTART:16010101T030000
TZOFFSETFROM:+0200
TZOFFSETTO:+0100
RRULE:FREQ=YEARLY;BYDAY=-1SU;BYMONTH=10
END:STANDARD
BEGIN:DAYLIGHT
DTSTART:16010101T020000
TZOFFSETFROM:+0100
TZOFFSETTO:+0200
RRULE:FREQ=YEARLY;BYDAY=-1SU;BYMONTH=3
END:DAYLIGHT
END:VTIMEZONE
END:VCALENDAR
`)).Decode()
	if err != nil {
		log.Fatal(err)
	}

	loc, err := tzatziki.ToLocation(cal.Children[0])
	if err != nil {
		log.Fatal(err)
	}

	// Every Monday at 09:00 stays at 09:00 when the clocks go back.
	monday := time.Date(2026, time.October, 19, 9, 0, 0, 0, loc)
	fmt.Println(monday.UTC().Format(time.RFC3339))
	fmt.Println(monday.AddDate(0, 0, 7).UTC().Format(time.RFC3339))
	// Output:
	// 2026-10-19T07:00:00Z
	// 2026-10-26T08:00:00Z
}

func ExampleResolver() {
	cal, err := ical.NewDecoder(strings.NewReader(`BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//example//EN
BEGIN:VTIMEZONE
TZID:Custom/Nowhere
BEGIN:STANDARD
DTSTART:19700101T000000
TZOFFSETFROM:+0530
TZOFFSETTO:+0530
END:STANDARD
END:VTIMEZONE
END:VCALENDAR
`)).Decode()
	if err != nil {
		log.Fatal(err)
	}
	resolver := tzatziki.NewResolver(cal)

	for _, tzid := range []string{"W. Europe Standard Time", "/freeassociation.sourceforge.net/America/New_York", "Custom/Nowhere", "Mars/Olympus"} {
		loc, err := resolver.Location(tzid)
		if err != nil {
			fmt.Println(err)
			continue
		}
		// A zone read from a definition has no name.
		line := time.Date(2026, time.November, 10, 9, 0, 0, 0, loc).Format("15:04 -0700 ") + loc.String()
		fmt.Println(strings.TrimSpace(line))
	}
	// Output:
	// 09:00 +0100 Europe/Berlin
	// 09:00 -0500 America/New_York
	// 09:00 +0530
	// tzatziki: unknown time zone "Mars/Olympus", and the calendar doesn't define it
}

func ExampleLoadLocation() {
	loc, err := tzatziki.LoadLocation("Tokyo Standard Time")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(loc)
	// Output: Asia/Tokyo
}

func ExampleWindowsToIANA() {
	name, ok := tzatziki.WindowsToIANA("Eastern Standard Time")
	fmt.Println(name, ok)
	// Output: America/New_York true
}
