package web

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/arschles/go-bindata-html-template"
	"github.com/elazarl/go-bindata-assetfs"

	"github.com/the-maldridge/NoobFarm2/internal/qdb"
	"github.com/the-maldridge/NoobFarm2/internal/web/assets"
)

var (
	bind = flag.String("web_bind", "", "Address to bind to")
	port = flag.Int("web_port", 8080, "Port to bind the webserver to")
	db   qdb.Backend
)

type PageConfig struct {
	Page                int
	Pages               int
	Quotes              []qdb.Quote
	DBSize              int
	ModerationQueueSize int
	NextButton          bool
	PrevButton          bool
	NextLink            string
	PrevLink            string
	SortConfig          qdb.SortConfig
}

func Serve(quotedb qdb.Backend) {
	db = quotedb
	http.HandleFunc("/", HomePage)
	http.HandleFunc("/viewquote.php", HomePage)
	http.HandleFunc("/add", AddQuote)
	http.HandleFunc("/status", StatusPage)

	http.Handle("/static/",
		http.StripPrefix("/static/",
			http.FileServer(
				&assetfs.AssetFS{
					Asset:     assets.Asset,
					AssetDir:  assets.AssetDir,
					AssetInfo: assets.AssetInfo,
					Prefix:    "static",
				},
			),
		),
	)

	log.Fatal(http.ListenAndServe(fmt.Sprintf("%s:%d", *bind, *port), nil))
}

func StatusPage(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Server OK")
}

func HomePage(w http.ResponseWriter, r *http.Request) {
	t, err := template.New("HomePage", assets.Asset).Parse("templates/home.tmpl")
	if err != nil {
		fmt.Fprintf(w, "Template Parse Error!")
	}

	// Setup the page config
	p := PageConfig{
		DBSize:              db.Size(),
		ModerationQueueSize: db.ModerationQueueSize(),
	}

	params := r.URL.Query()
	if params["id"] != nil {
		// This is requesting a single quote
		n, err := strconv.ParseInt(params["id"][0], 10, 32)
		if err != nil {
			n = -1
		}
		q, err := db.GetQuote(int(n))
		if err != nil {
			p.Quotes = []qdb.Quote{}
		} else {
			p.Quotes = []qdb.Quote{q}
		}
	} else {
		// This is either a search or a generic request,
		// either way we need a sorting config so that should
		// be parsed out.
		req := parseSortConfig(params)
		p.Quotes, p.Pages = db.GetBulkQuotes(req)
		p.Page = req.Offset/req.Number + 1
		p.SortConfig = req
	}

	p.PrevButton = p.Page > 1
	p.NextButton = p.Pages > 0 && p.Page != p.Pages

	if p.PrevButton {
		p.PrevLink = navLink(p, -1)
	}
	if p.NextButton {
		p.NextLink = navLink(p, 1)
	}

	var page bytes.Buffer
	err = t.Execute(&page, p)
	if err != nil {
		fmt.Fprintf(w, "Template runtime error")
	}

	html := strings.Replace(page.String(), "\\n", "<br />", -1)
	fmt.Fprintf(w, html)
}

func AddQuote(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		r.ParseForm()
		webQuote, ok := r.Form["Quote"]
		if !ok {
			// Return bad request
			http.Error(w, "Quote field missing in request", 400)
			return
		}
		// Normalize newlines
		quote := strings.Replace(webQuote[0], "\r\n", "\\n", -1)

		// Build and save the quote
		q := qdb.Quote{
			Quote:       quote,
			Submitted:   time.Now(),
			SubmittedIP: r.RemoteAddr,
		}
		if err := db.NewQuote(q); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}

	// Not adding a quote, send the form instead
	t, err := template.New("NewQuote", assets.Asset).Parse("templates/add.tmpl")
	if err != nil {
		fmt.Fprintf(w, "Template Parse Error!")
	}
	t.Execute(w, nil)
}

func navLink(p PageConfig, offset int) string {
	method := ""
	direction := ""
	if p.SortConfig.Descending {
		direction = "down"
	} else {
		direction = "up"
	}
	if p.SortConfig.ByRating {
		method = "rating"
	} else {
		method = "date"
	}

	return fmt.Sprintf("/?count=%d&page=%d&sort_by=%s&sort_order=%s",
		p.SortConfig.Number,
		p.Page+offset,
		method,
		direction,
	)
}

func parseSortConfig(params url.Values) qdb.SortConfig {
	req := qdb.SortConfig{
		ByDate:     true,
		Descending: true,
		Number:     10,
	}

	if params["count"] != nil {
		n, err := strconv.ParseInt(params["count"][0], 10, 32)
		if err != nil {
			req.Number = 10
		}
		req.Number = int(n)
	}

	if params["page"] != nil {
		n, err := strconv.ParseInt(params["page"][0], 10, 32)
		if err != nil {
			req.Offset = 0
		}
		req.Offset = int(n-1) * req.Number
		if req.Offset < 0 {
			req.Offset = 0
		}
	}

	if params["sort_by"] != nil {
		if params["sort_by"][0] == "rating" {
			req.ByRating = true
			req.ByDate = false
		}
	}

	if params["sort_order"] != nil {
		if params["sort_order"][0] == "down" {
			req.Descending = true
		} else {
			req.Descending = false
		}
	}
	return req
}
