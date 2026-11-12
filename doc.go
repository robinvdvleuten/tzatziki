// Package tzatziki converts between Go's [time.Location] and the VTIMEZONE
// components of iCalendar (RFC 5545).
//
// [FromLocation] writes a VTIMEZONE with one observance for each real clock
// change in a range, so no recurrence rule has to be inferred. [ToLocation]
// reads a VTIMEZONE, from any sender, back into a real [time.Location], so
// times in it keep their wall clock across clock changes just like in a named
// zone. [Resolver] resolves the TZID of a property the way invites need: a
// Windows zone name, then an IANA name, then the definition the calendar
// carries.
//
// IANA names are loaded with [time.LoadLocation]. A program that runs where
// there may be no zone files can import [time/tzdata] to carry its own.
package tzatziki
