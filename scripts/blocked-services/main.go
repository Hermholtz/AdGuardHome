// blocked services fetches the most recent Hostlists Registry blocked service
// index and transforms the filters from it to AdGuard Home's data and code
// formats.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"text/template"
	"time"

	"github.com/AdguardTeam/golibs/errors"
	"github.com/AdguardTeam/golibs/logutil/slogutil"
)

func main() {
	ctx := context.Background()
	l := slogutil.New(nil)

	urlStr := "https://adguardteam.github.io/HostlistsRegistry/assets/services.json"
	if v, ok := os.LookupEnv("URL"); ok {
		urlStr = v
	}

	// Validate the URL.
	_, err := url.Parse(urlStr)
	errors.Check(err)

	c := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp := errors.Must(c.Get(urlStr))
	defer slogutil.CloseAndLog(ctx, l, resp.Body, slog.LevelError)

	if resp.StatusCode != http.StatusOK {
		panic(fmt.Errorf("expected code %d, got %d", http.StatusOK, resp.StatusCode))
	}

	hlSvcs := &hlServices{}
	err = json.NewDecoder(resp.Body).Decode(hlSvcs)
	errors.Check(err)

	// Sort all services and rules to make the output more predictable.
	slices.SortStableFunc(hlSvcs.BlockedServices, func(a, b *hlServicesService) (res int) {
		return strings.Compare(a.ID, b.ID)
	})
	for _, s := range hlSvcs.BlockedServices {
		slices.Sort(s.Rules)
	}

	// Use another set of delimiters to prevent them interfering with the Go
	// code.
	tmpl, err := template.New("main").Delims("<%", "%>").Funcs(template.FuncMap{
		"isnotlast": func(idx, sliceLen int) (ok bool) {
			return idx != sliceLen-1
		},
	}).Parse(tmplStr)
	errors.Check(err)

	f := errors.Must(os.OpenFile(
		"./internal/filtering/servicelist.go",
		os.O_CREATE|os.O_TRUNC|os.O_WRONLY,
		0o644,
	))
	defer slogutil.CloseAndLog(ctx, l, f, slog.LevelError)

	errors.Check(tmpl.Execute(f, hlSvcs))
}

// tmplStr is the template for the Go source file with the services.
const tmplStr = `// Code generated by go run ./scripts/blocked-services/main.go; DO NOT EDIT.

package filtering

// blockedService represents a single blocked service.
type blockedService struct {
	ID      string   ` + "`" + `json:"id"` + "`" + `
	Name    string   ` + "`" + `json:"name"` + "`" + `
	IconSVG []byte   ` + "`" + `json:"icon_svg"` + "`" + `
	Rules   []string ` + "`" + `json:"rules"` + "`" + `
}

// blockedServices contains raw blocked service data.
var blockedServices = []blockedService{<% $l := len .BlockedServices %>
	<%- range $i, $s := .BlockedServices %>{
	ID:      <% printf "%q" $s.ID %>,
	Name:    <% printf "%q" $s.Name %>,
	IconSVG: []byte(<% printf "%q" $s.IconSVG %>),
	Rules: []string{<% range $s.Rules %>
		<% printf "%q" . %>,<% end %>
	},
}<% if isnotlast $i $l %>, <% end %><% end %>}
`

// hlServices is the JSON structure for the Hostlists Registry blocked service
// index.
type hlServices struct {
	BlockedServices []*hlServicesService `json:"blocked_services"`
}

// hlServicesService is the JSON structure for a service in the Hostlists
// Registry.
type hlServicesService struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	IconSVG string   `json:"icon_svg"`
	Rules   []string `json:"rules"`
}
