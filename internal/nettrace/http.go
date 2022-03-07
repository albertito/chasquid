package nettrace

import (
	"bytes"
	"embed"
	"fmt"
	"hash/crc32"
	"html/template"
	"math"
	"net/http"
	"sort"
	"strconv"
	"time"
)

//go:embed "templates/*.tmpl" "templates/*.css"
var templatesFS embed.FS
var top *template.Template

func init() {
	top = template.Must(
		template.New("_top").Funcs(template.FuncMap{
			"stripZeros":    stripZeros,
			"roundSeconds":  roundSeconds,
			"roundDuration": roundDuration,
			"colorize":      colorize,
			"depthspan":     depthspan,
			"shorttitle":    shorttitle,
			"traceemoji":    traceemoji,
		}).ParseFS(templatesFS, "templates/*"))
}

// RegisterHandler registers a the trace handler in the given ServeMux, on
// `/debug/traces`.
func RegisterHandler(mux *http.ServeMux) {
	mux.HandleFunc("/debug/traces", RenderTraces)
}

// RenderTraces is an http.Handler that renders the tracing information.
func RenderTraces(w http.ResponseWriter, req *http.Request) {
	data := &struct {
		Buckets   *[]time.Duration
		FamTraces map[string]*familyTraces

		// When displaying traces for a specific family.
		Family    string
		Bucket    int
		BucketStr string
		AllGT     bool
		Traces    []*trace

		// When displaying latencies for a specific family.
		Latencies *histSnapshot

		// When displaying a specific trace.
		Trace     *trace
		AllEvents []traceAndEvent

		// Error to show to the user.
		Error string
	}{}

	// Reference the common buckets, no need to copy them.
	data.Buckets = &buckets

	// Copy the family traces map, so we don't have to keep it locked for too
	// long. We'll still need to lock individual entries.
	data.FamTraces = copyFamilies()

	// Default to showing greater-than.
	data.AllGT = true
	if all := req.FormValue("all"); all != "" {
		data.AllGT, _ = strconv.ParseBool(all)
	}

	// Fill in the family related parameters.
	if fam := req.FormValue("fam"); fam != "" {
		if _, ok := data.FamTraces[fam]; !ok {
			data.Family = ""
			data.Error = "Unknown family"
			w.WriteHeader(http.StatusNotFound)
			goto render
		}
		data.Family = fam

		if bs := req.FormValue("b"); bs != "" {
			i, err := strconv.Atoi(bs)
			if err != nil {
				data.Error = "Invalid bucket (not a number)"
				w.WriteHeader(http.StatusBadRequest)
				goto render
			} else if i < -2 || i >= nBuckets {
				data.Error = "Invalid bucket number"
				w.WriteHeader(http.StatusBadRequest)
				goto render
			}
			data.Bucket = i
			data.Traces = data.FamTraces[data.Family].TracesFor(i, data.AllGT)

			switch i {
			case -2:
				data.BucketStr = "errors"
			case -1:
				data.BucketStr = "active"
			default:
				data.BucketStr = buckets[i].String()
			}
		}
	}

	if lat := req.FormValue("lat"); data.Family != "" && lat != "" {
		data.Latencies = data.FamTraces[data.Family].Latencies()
	}

	if traceID := req.FormValue("trace"); traceID != "" {
		refID := req.FormValue("ref")
		tr := findInFamilies(id(traceID), id(refID))
		if tr == nil {
			data.Error = "Trace not found"
			w.WriteHeader(http.StatusNotFound)
			goto render
		}
		data.Trace = tr
		data.Family = tr.Family
		data.AllEvents = allEvents(tr)
	}

render:

	// Write into a buffer, to avoid accidentally holding a lock on http
	// writes. It shouldn't happen, but just to be extra safe.
	bw := &bytes.Buffer{}
	bw.Grow(16 * 1024)
	err := top.ExecuteTemplate(bw, "index.html.tmpl", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		panic(err)
	}

	w.Write(bw.Bytes())
}

type traceAndEvent struct {
	Trace *trace
	Event event
	Depth uint
}

// allEvents gets all the events for the trace and its children/linked traces;
// and returns them sorted by timestamp.
func allEvents(tr *trace) []traceAndEvent {
	// Map tracking all traces we've seen, to avoid loops.
	seen := map[id]bool{}

	// Recursively gather all events.
	evts := appendAllEvents(tr, []traceAndEvent{}, seen, 0)

	// Sort them by time.
	sort.Slice(evts, func(i, j int) bool {
		return evts[i].Event.When.Before(evts[j].Event.When)
	})

	return evts
}

func appendAllEvents(tr *trace, evts []traceAndEvent, seen map[id]bool, depth uint) []traceAndEvent {
	if seen[tr.ID] {
		return evts
	}
	seen[tr.ID] = true

	subTraces := []*trace{}

	// Append all events of this trace.
	trevts := tr.Events()
	for _, e := range trevts {
		evts = append(evts, traceAndEvent{tr, e, depth})
		if e.Ref != nil {
			subTraces = append(subTraces, e.Ref)
		}
	}

	for _, t := range subTraces {
		evts = appendAllEvents(t, evts, seen, depth+1)
	}

	return evts
}

func stripZeros(d time.Duration) string {
	if d < time.Second {
		_, frac := math.Modf(d.Seconds())
		return fmt.Sprintf(" .%6d", int(frac*1000000))
	}
	return fmt.Sprintf("%.6f", d.Seconds())
}

func roundSeconds(d time.Duration) string {
	return fmt.Sprintf("%.6f", d.Seconds())
}

func roundDuration(d time.Duration) time.Duration {
	return d.Round(time.Millisecond)
}

func colorize(depth uint, id id) template.CSS {
	if depth == 0 {
		return template.CSS("rgba(var(--text-color))")
	}

	if depth > 3 {
		depth = 3
	}

	// Must match the number of nested color variables in the CSS.
	colori := crc32.ChecksumIEEE([]byte(id)) % 6
	return template.CSS(
		fmt.Sprintf("var(--nested-d%02d-c%02d)", depth, colori))
}

func depthspan(depth uint) template.HTML {
	s := `<span class="depth">`
	switch depth {
	case 0:
	case 1:
		s += "· "
	case 2:
		s += "· · "
	case 3:
		s += "· · · "
	case 4:
		s += "· · · · "
	default:
		s += fmt.Sprintf("· (%d) · ", depth)
	}

	s += `</span>`
	return template.HTML(s)
}

// Hand-picked emojis that have enough visual differences in most common
// renderings, and are common enough to be able to easily describe them.
var emojids = []rune(`😀🤣😇🥰🤧😈🤡👻👽🤖👋✊🦴👅` +
	`🐒🐕🦊🐱🐯🐎🐄🐷🐑🐐🐪🦒🐘🐀🦇🐓🦆🦚🦜🐢🐍🦖🐋🐟🦈🐙` +
	`🦋🐜🐝🪲🌻🌲🍉🍌🍍🍎🍑🥕🍄` +
	`🧀🍦🍰🧉🚂🚗🚜🛵🚲🛼🪂🚀🌞🌈🌊⚽`)

func shorttitle(tr *trace) string {
	all := tr.Family + " - " + tr.Title
	if len(all) > 20 {
		all = "..." + all[len(all)-17:]
	}
	return all
}

func traceemoji(id id) string {
	i := crc32.ChecksumIEEE([]byte(id)) % uint32(len(emojids))
	return string(emojids[i])
}
