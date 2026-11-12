package tzatziki

//go:generate go run gen_windowszones.go

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-ical"
)

// ErrUnknownZone means a TZID is no zone name this package knows, and no
// definition was found for it.
var ErrUnknownZone = errors.New("unknown time zone")

// WindowsToIANA returns the IANA name for a Windows time zone name, such as
// "W. Europe Standard Time", from CLDR's table. It reports false for any
// other name.
func WindowsToIANA(name string) (string, bool) {
	iana, ok := windowsZones[name]
	return iana, ok
}

// LoadLocation returns the zone a TZID names, without looking at any
// definition. It takes a Windows zone name, an IANA name, or an IANA name with
// a libical prefix, as in "/freeassociation.sourceforge.net/Europe/Paris".
// "Local" and the empty name are refused, because they would mean the zone of
// whichever machine runs this, or UTC by accident.
func LoadLocation(tzid string) (*time.Location, error) {
	if loc := namedLocation(tzid); loc != nil {
		return loc, nil
	}
	return nil, fmt.Errorf("tzatziki: %w %q", ErrUnknownZone, tzid)
}

func namedLocation(tzid string) *time.Location {
	if name, ok := windowsZones[tzid]; ok {
		tzid = name
	}
	if tzid == "" || tzid == "Local" {
		return nil
	}
	if strings.HasPrefix(tzid, "/") {
		parts := strings.Split(strings.TrimPrefix(tzid, "/"), "/")
		for i := 1; i < len(parts); i++ {
			if loc, err := time.LoadLocation(strings.Join(parts[i:], "/")); err == nil {
				return loc
			}
		}
		return nil
	}
	loc, err := time.LoadLocation(tzid)
	if err != nil {
		return nil
	}
	return loc
}

// Resolver resolves the TZIDs used in one calendar. A Resolver is safe for
// concurrent use.
type Resolver struct {
	definitions map[string]*ical.Component

	mu       sync.Mutex
	resolved map[string]resolved
}

type resolved struct {
	loc *time.Location
	err error
}

// NewResolver returns a Resolver for the VTIMEZONE components in cal, which
// may be nil.
func NewResolver(cal *ical.Calendar) *Resolver {
	r := &Resolver{
		definitions: map[string]*ical.Component{},
		resolved:    map[string]resolved{},
	}
	if cal == nil {
		return r
	}
	for _, child := range cal.Children {
		if child.Name == ical.CompTimezone {
			if prop := child.Props.Get(ical.PropTimezoneID); prop != nil {
				r.definitions[prop.Value] = child
			}
		}
	}
	return r
}

// Location resolves a TZID: a name as [LoadLocation] takes it first, then the
// calendar's own definition, read by [ToLocation]. A name wins because it
// stays correct when the sender's definition is out of date. A TZID that
// neither knows fails with [ErrUnknownZone]; it is never guessed.
//
// Each TZID is resolved once, and its result, or its error, is kept.
func (r *Resolver) Location(tzid string) (*time.Location, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if result, ok := r.resolved[tzid]; ok {
		return result.loc, result.err
	}
	loc, err := r.find(tzid)
	r.resolved[tzid] = resolved{loc, err}
	return loc, err
}

func (r *Resolver) find(tzid string) (*time.Location, error) {
	if loc := namedLocation(tzid); loc != nil {
		return loc, nil
	}
	if component, ok := r.definitions[tzid]; ok {
		return ToLocation(component)
	}
	return nil, fmt.Errorf("tzatziki: %w %q, and the calendar doesn't define it", ErrUnknownZone, tzid)
}
