package marketdata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"prediction-market-arb-bot/internal/config"
	"prediction-market-arb-bot/internal/types"
)

func TestFetchCandidatesPaginatesUntilShortPage(t *testing.T) {
	var offsets []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/events" {
			http.NotFound(w, r)
			return
		}
		offsets = append(offsets, r.URL.Query().Get("offset"))
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("offset") {
		case "0":
			_, _ = w.Write([]byte(`[
				{"slug":"ev1","title":"event 1","tags":[{"slug":"politics","label":"Politics"}],"markets":[
					{"slug":"m1","question":"q1","enableOrderBook":true,"liquidityNum":10000,"volume24hr":100,"outcomes":"[\"Yes\",\"No\"]","outcomePrices":"[\"0.6\",\"0.4\"]","clobTokenIds":"[\"tok1\",\"tok2\"]"},
					{"slug":"m2","question":"q2","enableOrderBook":true,"liquidityNum":10000,"volume24hr":100,"outcomes":"[\"Yes\",\"No\"]","outcomePrices":"[\"0.7\",\"0.3\"]","clobTokenIds":"[\"tok3\",\"tok4\"]"}
				]},
				{"slug":"ev2","title":"event 2","tags":[{"slug":"sports","label":"Sports"}],"markets":[
					{"slug":"m3","question":"q3","enableOrderBook":true,"liquidityNum":10000,"volume24hr":100,"outcomes":"[\"Yes\",\"No\"]","outcomePrices":"[\"0.8\",\"0.2\"]","clobTokenIds":"[\"tok5\",\"tok6\"]"}
				]}
			]`))
		case "2":
			_, _ = w.Write([]byte(`[
				{"slug":"ev3","title":"event 3","tags":[{"slug":"ai","label":"AI"}],"markets":[
					{"slug":"m4","question":"q4","enableOrderBook":true,"liquidityNum":10000,"volume24hr":100,"outcomes":"[\"Yes\",\"No\"]","outcomePrices":"[\"0.65\",\"0.35\"]","clobTokenIds":"[\"tok7\",\"tok8\"]"}
				]}
			]`))
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	defer server.Close()

	cfg := config.Load()
	cfg.GammaBaseURL = server.URL
	cfg.EventsPerPage = 2
	cfg.EventPagesPerSort = 2
	cfg.MinMarketLiquidityUSD = 5000
	client := New(cfg)
	candidates, err := client.FetchCandidates(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 4 {
		t.Fatalf("expected 4 candidates, got %d", len(candidates))
	}
	if len(offsets) < 2 || offsets[0] != "0" || offsets[1] != "2" {
		t.Fatalf("unexpected offsets: %#v", offsets)
	}
}

func TestFetchCandidateSetDefaultPagesUntilShortPage(t *testing.T) {
	var offsets []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/events" {
			http.NotFound(w, r)
			return
		}
		offsets = append(offsets, r.URL.Query().Get("offset"))
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("offset") {
		case "0":
			_, _ = w.Write([]byte(`[
				{"slug":"ev1","title":"event 1","tags":[{"slug":"politics","label":"Politics"}],"markets":[
					{"slug":"m1","question":"q1","enableOrderBook":true,"liquidityNum":10000,"volume24hr":100,"outcomes":"[\"Yes\",\"No\"]","outcomePrices":"[\"0.6\",\"0.4\"]","clobTokenIds":"[\"tok1\",\"tok2\"]"}
				]}
			]`))
		case "1":
			_, _ = w.Write([]byte(`[
				{"slug":"ev2","title":"event 2","tags":[{"slug":"politics","label":"Politics"}],"markets":[
					{"slug":"m2","question":"q2","enableOrderBook":true,"liquidityNum":10000,"volume24hr":100,"outcomes":"[\"Yes\",\"No\"]","outcomePrices":"[\"0.7\",\"0.3\"]","clobTokenIds":"[\"tok3\",\"tok4\"]"}
				]}
			]`))
		case "2":
			_, _ = w.Write([]byte(`[]`))
		default:
			t.Fatalf("unexpected offset %s", r.URL.Query().Get("offset"))
		}
	}))
	defer server.Close()

	cfg := config.Load()
	cfg.GammaBaseURL = server.URL
	cfg.EventsPerPage = 1
	cfg.EventPagesPerSort = 0
	cfg.MinMarketLiquidityUSD = 5000
	client := New(cfg)
	candidates, report, err := client.FetchCandidateSet(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 2 {
		t.Fatalf("expected 2 unique candidates, got %d", len(candidates))
	}
	if len(offsets) < 3 || offsets[0] != "0" || offsets[1] != "1" || offsets[2] != "2" {
		t.Fatalf("expected pagination until empty page, got offsets=%#v", offsets)
	}
	if len(report.SortDimensions) != len(eventSortDimensions) || !report.SortDimensions[0].ShortPage {
		t.Fatalf("expected discovery report with short-page evidence, got %#v", report.SortDimensions)
	}
}

func TestFetchCandidateSetReturnsPartialReportOnPageFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/events" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("offset") == "1" {
			http.Error(w, "boom", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"slug":"ev1","title":"event 1","tags":[{"slug":"politics","label":"Politics"}],"markets":[
				{"slug":"m1","question":"q1","enableOrderBook":true,"liquidityNum":10000,"volume24hr":100,"outcomes":"[\"Yes\",\"No\"]","outcomePrices":"[\"0.6\",\"0.4\"]","clobTokenIds":"[\"tok1\",\"tok2\"]"}
			]}
		]`))
	}))
	defer server.Close()

	cfg := config.Load()
	cfg.GammaBaseURL = server.URL
	cfg.EventsPerPage = 1
	cfg.EventPagesPerSort = 0
	client := New(cfg)
	candidates, report, err := client.FetchCandidateSet(context.Background())
	if err == nil {
		t.Fatal("expected page failure")
	}
	if len(candidates) != 1 || report.AcceptedMarketCount != 1 || report.RawEventCount != 1 {
		t.Fatalf("expected partial candidates/report, candidates=%d report=%#v", len(candidates), report)
	}
	if len(report.SortDimensions) != 1 || report.SortDimensions[0].Pages != 1 || report.SortDimensions[0].Events != 1 {
		t.Fatalf("expected partial sort dimension progress, got %#v", report.SortDimensions)
	}
}

func TestFetchCandidatesCapsEventsPageSizeAtGammaLimit(t *testing.T) {
	var limits []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limits = append(limits, r.URL.Query().Get("limit"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	cfg := config.Load()
	cfg.GammaBaseURL = server.URL
	cfg.EventsPerPage = 500
	cfg.EventPagesPerSort = 1
	client := New(cfg)
	if _, err := client.FetchCandidates(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(limits) != len(eventSortDimensions) || limits[0] != "50" {
		t.Fatalf("expected capped limit=50, got %#v", limits)
	}
}

func TestCandidateFiltersRejectKnownJunk(t *testing.T) {
	cfg := config.Load()
	cfg.MinMarketLiquidityUSD = 5000
	cases := []struct {
		name      string
		candidate types.MarketCandidate
		code      string
	}{
		{
			name: "disabled order book",
			candidate: baseCandidate(func(c *types.MarketCandidate) {
				c.EnableOrderBook = false
			}),
			code: "order_book_disabled",
		},
		{
			name: "low liquidity",
			candidate: baseCandidate(func(c *types.MarketCandidate) {
				c.LiquidityUSD = 100
			}),
			code: "below_min_liquidity",
		},
		{
			name: "missing prices",
			candidate: baseCandidate(func(c *types.MarketCandidate) {
				c.OutcomePrices = nil
			}),
			code: "missing_outcome_prices",
		},
		{
			name: "extreme price",
			candidate: baseCandidate(func(c *types.MarketCandidate) {
				c.OutcomePrices = []float64{0.99, 0.01}
			}),
			code: "extreme_price",
		},
		{
			name: "coin flip",
			candidate: baseCandidate(func(c *types.MarketCandidate) {
				c.OutcomePrices = []float64{0.50, 0.50}
			}),
			code: "coin_flip_prices",
		},
		{
			name: "price keyword",
			candidate: baseCandidate(func(c *types.MarketCandidate) {
				c.Question = "Bitcoin hits $200k tomorrow?"
			}),
			code: "price_prediction_keyword",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			ok, notes := candidatePassesFilters(tt.candidate, cfg)
			if ok || !contains(notes, tt.code) {
				t.Fatalf("expected %s in notes, got ok=%v notes=%#v", tt.code, ok, notes)
			}
		})
	}
}

func baseCandidate(mutate func(*types.MarketCandidate)) types.MarketCandidate {
	c := types.MarketCandidate{
		MarketSlug:      "m",
		Question:        "Will X happen?",
		TokenIDs:        []string{"yes", "no"},
		OutcomePrices:   []float64{0.65, 0.35},
		LiquidityUSD:    10000,
		Volume24hUSD:    100,
		EnableOrderBook: true,
		CategorySlug:    "politics",
	}
	mutate(&c)
	return c
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func TestBestPricesDoNotDependOnBookOrder(t *testing.T) {
	bid, ok := bestBid([]types.BookLevel{
		{Price: 0.01, Size: 10},
		{Price: 0.50, Size: 10},
		{Price: 0.20, Size: 10},
	})
	if !ok || bid != 0.50 {
		t.Fatalf("expected max bid, got %v ok=%v", bid, ok)
	}

	ask, ok := bestAsk([]types.BookLevel{
		{Price: 0.99, Size: 10},
		{Price: 0.51, Size: 10},
		{Price: 0.70, Size: 10},
	})
	if !ok || ask != 0.51 {
		t.Fatalf("expected min ask, got %v ok=%v", ask, ok)
	}
}
