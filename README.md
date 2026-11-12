# tzatziki

tzatziki converts between Go's `time.Location` and the `VTIMEZONE` components of iCalendar. You can write a recurring event in its real zone, so that "every Monday at 09:00" stays at 09:00 when the clocks change, and read the zones other programs send you, including Outlook's. Instead of guessing a recurrence rule from Go's zone data, it lists every real clock change, so what it writes reads back to the same offsets. It was extracted from [pim-mcp](https://github.com/robinvdvleuten/pim-mcp), which reads invites from strangers.

- **Exact round trip.** A zone written and read back is tested hour by hour against Go's own offsets, for eight zones over eleven years from three start years, including Lord Howe's half-hour change and Casablanca.
- **One observance per clock change.** No rule is inferred, so a reader that has only the definition gets the right offset at every moment the range covers.
- **Real zones from definitions.** A `VTIMEZONE` becomes a `time.Location` with all its transitions, so date arithmetic in it works like in a named zone.
- **Outlook's zones.** Rules that start in 1601, as Outlook writes them, are read correctly, and its Windows names such as "W. Europe Standard Time" are mapped through CLDR's table of 142 names.
- **Names first.** A TZID is resolved as a Windows name, then an IANA name (with or without a libical prefix), then the calendar's own definition. A name wins because the sender's definition may be out of date. An unknown zone is an error, never a guess.
- **Safe on hostile input.** Rule expansion has a step limit, a rule that repeats every second fails fast, and the reader is fuzzed.
- **Two dependencies.** [go-ical](https://github.com/emersion/go-ical) and [rrule-go](https://github.com/teambition/rrule-go).

```go
amsterdam, _ := time.LoadLocation("Europe/Amsterdam")
from := time.Date(2026, time.January, 1, 0, 0, 0, 0, amsterdam)
vtimezone, _ := tzatziki.FromLocation(amsterdam, from, from.AddDate(1, 0, 0))

loc, _ := tzatziki.ToLocation(vtimezone)
monday := time.Date(2026, time.October, 19, 9, 0, 0, 0, loc)
fmt.Println(monday.UTC().Format(time.Kitchen), monday.AddDate(0, 0, 7).UTC().Format(time.Kitchen))
// 7:00AM 8:00AM
```

---

## Getting started

You need [Go](https://go.dev/dl/) 1.26 or later.

```bash
go get github.com/robinvdvleuten/tzatziki
```

**Write a zone.** Give `FromLocation` a range that covers every time you write in the zone. After the range, a reader that has only the definition keeps the last offset. Add the result to the calendar beside the events that use its TZID.

```go
vtimezone, err := tzatziki.FromLocation(loc, start, start.AddDate(10, 0, 0))
cal.Children = append([]*ical.Component{vtimezone}, cal.Children...)
```

**Read the zones of a calendar.** Make one `Resolver` per calendar, and ask it for the TZID of each property:

```go
resolver := tzatziki.NewResolver(cal)
loc, err := resolver.Location(prop.Params.Get(ical.ParamTimezoneID))
```

**Read one definition** with `tzatziki.ToLocation(component)`, or **one name** with `tzatziki.LoadLocation(tzid)`. `tzatziki.WindowsToIANA(name)` looks up a Windows name alone.

A zone read from a definition has an empty name, because a definition carries no IANA name.

IANA names are loaded with `time.LoadLocation`. If your program may run without zone files, such as in a small container, import `time/tzdata` to carry them in the binary.

---

## Limits

- **Rules in definitions** are followed from 1900 to 2100. Before and after, the nearest offset holds.
- **A rule** may take at most 5000 steps per observance, and a definition at most 256 different offsets.
- **A written definition** may hold a few thousand clock changes. A longer range is an error.

---

## Development

Install [mise](https://mise.jdx.dev), which fetches the pinned Go, linter and Vale, then set up the commit hook:

```bash
mise install && mise run setup
```

Check a change: formatting, tests, lint and known vulnerabilities.

```bash
mise run check
```

The Windows zone table is generated from CLDR with `go generate` and committed.

## License

Copyright 2026 Robin van der Vleuten

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the License. You may obtain a copy of the License at

```
http://www.apache.org/licenses/LICENSE-2.0
```

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific language governing permissions and limitations under the License.
