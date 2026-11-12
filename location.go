package tzatziki

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-ical"
	"github.com/teambition/rrule-go"
)

// localFormat is a DATE-TIME without a zone, as in a DTSTART.
const localFormat = "20060102T150405"

// maxZoneSteps bounds each observance in a VTIMEZONE, which may come from
// strangers. Outlook starts its rules in 1601, which takes at most 200 yearly
// steps once moved to 1900.
const maxZoneSteps = 5000

// A rule in a definition is expanded over these years only. Before and after,
// the nearest offset holds.
var (
	zoneRulesStart = time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)
	zoneRulesEnd   = time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
)

// ErrRuleTooExpensive means an observance's RRULE repeats too often to expand,
// such as every second.
var ErrRuleTooExpensive = errors.New("time zone rule repeats too often to expand")

var errRuleUnusable = errors.New("the rule can't be worked out")

type zoneTransition struct {
	at     time.Time
	offset int
	isDST  bool
}

// ToLocation turns a VTIMEZONE into a real Location, so that times in it keep
// their wall clock across clock changes just like in a named zone. Its name is
// empty, because a definition has no IANA name.
//
// Observances may repeat by RRULE and RDATE. A rule is followed from 1900 to
// 2100, so one that starts in 1601, as Outlook writes them, works. A rule that
// would take more than a few thousand steps fails with [ErrRuleTooExpensive].
func ToLocation(component *ical.Component) (*time.Location, error) {
	if component == nil || component.Name != ical.CompTimezone {
		return nil, errors.New("tzatziki: not a VTIMEZONE")
	}
	loc, err := location(component)
	if err != nil {
		tzid := ""
		if prop := component.Props.Get(ical.PropTimezoneID); prop != nil {
			tzid = prop.Value
		}
		return nil, fmt.Errorf("tzatziki: time zone %q: %w", tzid, err)
	}
	return loc, nil
}

func location(component *ical.Component) (*time.Location, error) {
	var transitions []zoneTransition
	var first time.Time
	var firstOffset int
	for _, child := range component.Children {
		if child.Name != ical.CompTimezoneStandard && child.Name != ical.CompTimezoneDaylight {
			continue
		}
		from, err := utcOffset(child, ical.PropTimezoneOffsetFrom)
		if err != nil {
			return nil, err
		}
		to, err := utcOffset(child, ical.PropTimezoneOffsetTo)
		if err != nil {
			return nil, err
		}
		onsets, err := observanceOnsets(child, from)
		if err != nil {
			return nil, err
		}
		for _, onset := range onsets {
			at := onset.Add(-time.Duration(from) * time.Second)
			if first.IsZero() || at.Before(first) {
				first, firstOffset = at, from
			}
			transitions = append(transitions, zoneTransition{at, to, child.Name == ical.CompTimezoneDaylight})
		}
	}
	if len(transitions) == 0 {
		return nil, errors.New("definition has no observances")
	}
	slices.SortStableFunc(transitions, func(a, b zoneTransition) int { return a.at.Compare(b.at) })

	data, err := encodeTZif(firstOffset, transitions)
	if err != nil {
		return nil, err
	}
	loc, err := time.LoadLocationFromTZData("", data)
	if err != nil {
		return nil, fmt.Errorf("build definition: %w", err)
	}
	return loc, nil
}

var offsetPattern = regexp.MustCompile(`^([+-])(\d{2})(\d{2})(\d{2})?$`)

func utcOffset(component *ical.Component, name string) (int, error) {
	prop := component.Props.Get(name)
	if prop == nil {
		return 0, fmt.Errorf("definition has no %s", name)
	}
	match := offsetPattern.FindStringSubmatch(prop.Value)
	if match == nil {
		return 0, fmt.Errorf("read %s: bad offset", name)
	}
	hours, _ := strconv.Atoi(match[2])
	minutes, _ := strconv.Atoi(match[3])
	seconds, _ := strconv.Atoi(match[4])
	offset := hours*3600 + minutes*60 + seconds
	if match[1] == "-" {
		offset = -offset
	}
	return offset, nil
}

// observanceOnsets returns when an observance starts, as wall clock times
// held in UTC.
func observanceOnsets(component *ical.Component, from int) ([]time.Time, error) {
	prop := component.Props.Get(ical.PropDateTimeStart)
	if prop == nil {
		return nil, errors.New("definition has no DTSTART")
	}
	start, err := time.ParseInLocation(localFormat, prop.Value, time.UTC)
	if err != nil {
		return nil, fmt.Errorf("read definition's DTSTART: %w", err)
	}
	onsets := []time.Time{start}

	for _, prop := range component.Props.Values(ical.PropRecurrenceDates) {
		for value := range strings.SplitSeq(prop.Value, ",") {
			at, err := time.ParseInLocation(localFormat, value, time.UTC)
			if err != nil {
				return nil, fmt.Errorf("read definition's RDATE: %w", err)
			}
			onsets = append(onsets, at)
		}
	}

	prop = component.Props.Get(ical.PropRecurrenceRule)
	if prop == nil {
		return onsets, nil
	}
	option, err := rrule.StrToROptionInLocation(prop.Value, time.UTC)
	if err != nil {
		return nil, fmt.Errorf("read definition's RRULE: %w", err)
	}
	// rrule-go stops without a word once a rule has run for 292 years, and Outlook
	// starts its rules in 1601. The rules repeat yearly, so a later start gives
	// the same transitions.
	option.Dtstart = start
	if start.Before(zoneRulesStart) {
		option.Dtstart = time.Date(zoneRulesStart.Year(), start.Month(), start.Day(), start.Hour(), start.Minute(), start.Second(), 0, time.UTC)
	}
	if !option.Until.IsZero() {
		// UNTIL is true UTC here, while the onsets are wall clock times.
		option.Until = option.Until.Add(time.Duration(from) * time.Second)
	}
	rule, err := rrule.NewRRule(*option)
	if err != nil {
		return nil, fmt.Errorf("read definition's RRULE: %w", err)
	}
	next := rule.Iterator()
	for step := 0; ; step++ {
		if step >= maxZoneSteps {
			return nil, ErrRuleTooExpensive
		}
		at, ok, err := nextInstance(next)
		if err != nil {
			return nil, fmt.Errorf("read definition's RRULE: %w", err)
		}
		if !ok || at.After(zoneRulesEnd) {
			return onsets, nil
		}
		onsets = append(onsets, at)
	}
}

// nextInstance asks a rule for its next instance. rrule-go panics on some rules
// it accepted, such as one starting in the year 0, and rules come from strangers.
func nextInstance(next rrule.Next) (at time.Time, ok bool, err error) {
	defer func() {
		if recover() != nil {
			at, ok, err = time.Time{}, false, errRuleUnusable
		}
	}()
	at, ok = next()
	return at, ok, nil
}

// encodeTZif writes the format time.LoadLocationFromTZData reads (RFC 9636).
// The standard library has no other way to make a Location with transitions.
func encodeTZif(firstOffset int, transitions []zoneTransition) ([]byte, error) {
	type zoneType struct {
		offset int
		isDST  bool
	}
	types := []zoneType{{offset: firstOffset}}
	indexes := make([]byte, len(transitions))
	for i, transition := range transitions {
		zone := zoneType{transition.offset, transition.isDST}
		index := slices.Index(types, zone)
		if index < 0 {
			if len(types) > 255 {
				return nil, errors.New("definition has too many offsets")
			}
			index = len(types)
			types = append(types, zone)
		}
		indexes[i] = byte(index) //nolint:gosec // Limited to 255 just above.
	}

	var out bytes.Buffer
	header := func(transitionCount, typeCount, charCount uint32) {
		out.WriteString("TZif2")
		out.Write(make([]byte, 15))
		for _, count := range []uint32{0, 0, 0, transitionCount, typeCount, charCount} {
			_ = binary.Write(&out, binary.BigEndian, count)
		}
	}
	// Version 2 keeps a block of 32-bit data first, which readers skip. It stays
	// empty because Outlook's transitions start in 1601.
	header(0, 0, 0)
	header(uint32(len(transitions)), uint32(len(types)), 1) //nolint:gosec // Bounded by the caps above.
	for _, transition := range transitions {
		_ = binary.Write(&out, binary.BigEndian, transition.at.Unix())
	}
	out.Write(indexes)
	for _, zone := range types {
		_ = binary.Write(&out, binary.BigEndian, int32(zone.offset)) //nolint:gosec // Limited by the offset pattern.
		isDST := byte(0)
		if zone.isDST {
			isDST = 1
		}
		out.Write([]byte{isDST, 0})
	}
	out.WriteByte(0) // The one empty abbreviation that every type points at.
	return out.Bytes(), nil
}
