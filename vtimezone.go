package tzatziki

import (
	"errors"
	"fmt"
	"time"

	"github.com/emersion/go-ical"
)

// maxTimezoneSteps stops the walk if a zone were ever to loop. A zone takes
// about three steps a year, so this covers thousands of years.
const maxTimezoneSteps = 10000

type zoneState struct {
	name   string
	offset int
	dst    bool
}

func stateAt(at time.Time) zoneState {
	name, offset := at.Zone()
	return zoneState{name, offset, at.IsDST()}
}

// FromLocation returns the VTIMEZONE for loc that RFC 5545 asks for beside any
// TZID. Its TZID is loc's name. It lists the zone's real clock changes from
// from up to until, one observance each, so no recurrence rule has to be
// inferred. The first observance only fixes the offset in force at from, which
// is cut to the second.
//
// A reader that has only this definition keeps the last offset after until.
// Choose a range that covers every time written in the zone.
func FromLocation(loc *time.Location, from, until time.Time) (*ical.Component, error) {
	if loc == nil {
		return nil, errors.New("tzatziki: no location")
	}
	first := from.In(loc).Truncate(time.Second)
	if !until.After(first) {
		return nil, errors.New("tzatziki: until must be after from")
	}

	vtz := ical.NewComponent(ical.CompTimezone)
	vtz.Props.SetText(ical.PropTimezoneID, loc.String())

	previous := stateAt(first)
	vtz.Children = append(vtz.Children, observance(first, previous.offset))
	at := first
	for range maxTimezoneSteps {
		_, next := at.ZoneBounds()
		switch {
		case next.IsZero() || !next.Before(until):
			return vtz, nil
		case !next.After(at):
			// Past the last year in the zone's data, Go answers from a rule, and on the
			// last day of a leap year its bound lies before the time asked about.
			// Moving on an hour gets out of that day, but the clocks can change in
			// that hour, so the exact second is searched for.
			later := at.Add(time.Hour)
			if stateAt(later) == previous {
				at = later
				continue
			}
			change := firstChange(at, later)
			vtz.Children = append(vtz.Children, observance(change, previous.offset))
			previous = stateAt(change)
			at = change
		default:
			// The same rule answers with the end of the year when nothing changes then.
			if state := stateAt(next); state != previous {
				vtz.Children = append(vtz.Children, observance(next, previous.offset))
				previous = state
			}
			at = next
		}
	}
	return nil, fmt.Errorf("tzatziki: time zone %s has too many clock changes, use a shorter range", loc)
}

// firstChange returns the first second after from at which the zone is no longer
// as it is at from. The zone must have changed by until.
func firstChange(from, until time.Time) time.Time {
	before := stateAt(from)
	for until.Sub(from) > time.Second {
		middle := from.Add(until.Sub(from) / 2).Truncate(time.Second)
		if stateAt(middle) == before {
			from = middle
		} else {
			until = middle
		}
	}
	return until
}

// observance describes the zone from the instant at, which must be the start of
// a period. Its DTSTART is in the offset before it, as RFC 5545 says.
func observance(at time.Time, offsetFrom int) *ical.Component {
	name, offsetTo := at.Zone()
	kind := ical.CompTimezoneStandard
	if at.IsDST() {
		kind = ical.CompTimezoneDaylight
	}
	component := ical.NewComponent(kind)
	start := ical.NewProp(ical.PropDateTimeStart)
	start.Value = at.In(time.FixedZone("", offsetFrom)).Format(localFormat)
	component.Props.Set(start)
	component.Props.Set(offsetProp(ical.PropTimezoneOffsetFrom, offsetFrom))
	component.Props.Set(offsetProp(ical.PropTimezoneOffsetTo, offsetTo))
	component.Props.SetText(ical.PropTimezoneName, name)
	return component
}

func offsetProp(name string, seconds int) *ical.Prop {
	sign := '+'
	if seconds < 0 {
		sign, seconds = '-', -seconds
	}
	prop := ical.NewProp(name)
	prop.Value = fmt.Sprintf("%c%02d%02d", sign, seconds/3600, seconds%3600/60)
	if seconds%60 != 0 {
		prop.Value += fmt.Sprintf("%02d", seconds%60)
	}
	return prop
}
