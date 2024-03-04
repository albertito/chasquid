//go:build !coverage
// +build !coverage

// Generate an HTML visualization of a Go coverage profile.
// Serves a similar purpose to "go tool cover -html", but has a different
// visual style.
package main

import (
	"flag"
	"fmt"
	"html/template"
	"math"
	"os"
	"strings"

	"golang.org/x/tools/cover"
)

var (
	input  = flag.String("input", "", "input file")
	output = flag.String("output", "", "output file")
	strip  = flag.Int("strip", 0, "how many path entries to strip")
	title  = flag.String("title", "Coverage report", "page title")
	notes  = flag.String("notes", "", "notes to add at the beginning (HTML)")
)

func errorf(f string, a ...interface{}) {
	fmt.Printf(f, a...)
	os.Exit(1)
}

func main() {
	flag.Parse()

	profiles, err := cover.ParseProfiles(*input)
	if err != nil {
		errorf("Error parsing input %q: %v\n", *input, err)
	}

	totals := &Totals{
		totalF:   map[string]int{},
		coveredF: map[string]int{},
	}
	files := []string{}
	code := map[string]template.HTML{}
	for _, p := range profiles {
		files = append(files, p.FileName)
		totals.Add(p)

		fname := strings.Join(strings.Split(p.FileName, "/")[*strip:], "/")
		src, err := os.ReadFile(fname)
		if err != nil {
			errorf("Failed to read %q: %v", fname, err)
		}

		code[p.FileName] = genHTML(src, p.Boundaries(src))
	}

	out, err := os.Create(*output)
	if err != nil {
		errorf("Failed to open output file %q: %v", *output, err)
	}

	data := struct {
		Title  string
		Notes  template.HTML
		Files  []string
		Code   map[string]template.HTML
		Totals *Totals
	}{
		Title:  *title,
		Notes:  template.HTML(*notes),
		Files:  files,
		Code:   code,
		Totals: totals,
	}

	tmpl := template.Must(template.New("html").Parse(htmlTmpl))
	err = tmpl.Execute(out, data)
	if err != nil {
		errorf("Failed to execute template: %v", err)
	}

	for _, f := range files {
		fmt.Printf("%5.1f%%  %v\n", totals.Percent(f), f)
	}
	fmt.Printf("\n")
	fmt.Printf("Total: %.1f\n", totals.TotalPercent())
}

// Totals is used to keep track of total counters.
type Totals struct {
	// Total statements.
	total int

	// Covered statements.
	covered int

	// Total statements per file.
	totalF map[string]int

	// Covered statements per file.
	coveredF map[string]int
}

// Add the given profile to the total counters.
func (t *Totals) Add(p *cover.Profile) {
	for _, b := range p.Blocks {
		t.total += b.NumStmt
		t.totalF[p.FileName] += b.NumStmt
		if b.Count > 0 {
			t.covered += b.NumStmt
			t.coveredF[p.FileName] += b.NumStmt
		}
	}
}

// Percent covered for the given file.
func (t *Totals) Percent(f string) float32 {
	return float32(t.coveredF[f]) / float32(t.totalF[f]) * 100
}

// TotalPercent covered, across all files.
func (t *Totals) TotalPercent() float32 {
	return float32(t.covered) / float32(t.total) * 100
}

func genHTML(src []byte, boundaries []cover.Boundary) template.HTML {
	// Position -> []Boundary
	// The order matters, we expect to receive start-end pairs in order, so
	// they are properly added.
	bs := map[int][]cover.Boundary{}
	for _, b := range boundaries {
		bs[b.Offset] = append(bs[b.Offset], b)
	}

	w := &strings.Builder{}
	for i := range src {
		// Emit boundary markers.
		for _, b := range bs[i] {
			if b.Start {
				n := 0
				if b.Count > 0 {
					n = int(math.Floor(b.Norm*4)) + 1
				}
				fmt.Fprintf(w, `<span class="cov%v" title="%v">`, n, b.Count)
			} else {
				w.WriteString("</span>")
			}
		}

		switch b := src[i]; b {
		case '>':
			w.WriteString("&gt;")
		case '<':
			w.WriteString("&lt;")
		case '&':
			w.WriteString("&amp;")
		case '\t':
			w.WriteString("        ")
		default:
			w.WriteByte(b)
		}
	}
	return template.HTML(w.String())
}

const htmlTmpl = `<!DOCTYPE html>
<html>
  <head>
    <meta charset="utf-8">
    <title>{{.Title}}</title>

    <style>
      :root {
        --txt: #333;
        --bg: white;
        --a: #1c3986;
        --a-hover: #069;
        --odd-bg: #f5f5f7;
        --pre-bg: #f8f8f8;
        --pre-border: #e5e5e5;

        // color palette from graphiq.
        --cov0: red;
        --cov1: #0b7bab;
        --cov2: #09639b;
        --cov3: #034a8b;
        --cov4: #00337c;
        --cov5: #032663;
      }

      @media (prefers-color-scheme: dark) {
        :root {
          --txt: rgba(255, 255, 255, 0.90);
          --bg: #121212;
          --a: #44b4ec;
          --a-hover: #7fc9ee;
          --odd-bg: #222222;
          --pre-bg: #111;
          --pre-border: #333;
          --cov0: #f2a359;
          --cov1: #2b84b1;
          --cov2: #3393c4;
          --cov3: #3ca3d7;
          --cov4: #44b3eb;
          --cov5: #4dc3ff;
        }
      }

      body {
        font: 100%/1.4 -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto,
          Oxygen, Ubuntu, Cantarell, "Fira Sans", "Droid Sans",
          "Helvetica Neue", Arial, sans-serif, "Apple Color Emoji",
          "Segoe UI Emoji", "Segoe UI Symbol";
        color: var(--txt);
        background: var(--bg);
      }

      h1 {
        margin: 0 0 0.5em;
      }

      a {
        color: var(--a);
        text-decoration: none;
        cursor: pointer;
      }

      a:hover {
        color: var(--a-hover);
      }

      table {
        border-collapse: collapse;
      }

      tr:nth-child(odd) {
        background: var(--odd-bg);
      }

      tr.total {
        border-top: 1px solid;
        font-weight: bold;
      }

      td {
        padding: 0.2em 1em;
      }

      td.pcnt {
        text-align: right;
      }

      code, pre, tt {
        font-family: Monaco, Bitstream Vera Sans Mono, Lucida Console,
          Terminal, Consolas, Liberation Mono, DejaVu Sans Mono,
          Courier New, monospace;
        color: var(--txt);
      }

      pre {
        padding: 0.5em 0.8em;
        background: var(--pre-bg);
        border-radius: 1em;
        border:1px solid var(--pre-border);
        overflow-x: auto;
      }

      .cov0 { color: var(--cov0); }
      .cov1 { color: var(--cov1); }
      .cov2 { color: var(--cov2); }
      .cov3 { color: var(--cov3); }
      .cov4 { color: var(--cov4); }
      .cov5 { color: var(--cov5); }
    </style>

    <script>
      function visible(id) {
        history.replaceState(undefined, undefined, "#" + id);
        var all = document.getElementsByClassName("file");
        for (var i = 0; i < all.length; i++) {
          var elem = all.item(i);
          elem.style.display = "none";
        }
        var chosen = document.getElementById(id);
        chosen.style.display = "block";
      }

      window.onload = function() {
        var id = window.location.hash.replace("#", "");
        if (id != "") {
          visible(id);
        }
      };
    </script>
  </head>

  <body>
  <h1>{{.Title}}</h1>

  {{.Notes}}<p>

  <table>
  {{range .Files}}
  <tr>
    <td><a onclick="visible('f::{{.}}')" tabindex="0"> {{.}} </a></td>
    <td class="pcnt">{{$.Totals.Percent . | printf "%.1f%%"}}</td>
  </tr>
  {{- end}}

  <tr class="total">
    <td>Total</td>
    <td class="pcnt">{{.Totals.TotalPercent | printf "%.1f"}}%</td>
  </tr>
  </table>

  <div id="source">
  {{range .Files}}
  <pre class="file" id="f::{{.}}" style="display: none">{{index $.Code .}}</pre>
  {{end}}
  </div>

  </body>
</html>
`
