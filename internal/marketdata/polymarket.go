package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"prediction-market-arb-bot/internal/config"
	"prediction-market-arb-bot/internal/types"
)

type Client struct {
	cfg  config.Config
	http *http.Client
}

func New(cfg config.Config) Client {
	return Client{cfg: cfg, http: &http.Client{Timeout: 20 * time.Second}}
}

func (c Client) FetchCandidates(ctx context.Context) ([]types.MarketCandidate, error) {
	u, err := url.Parse(c.cfg.GammaBaseURL + "/markets")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("active", "true")
	q.Set("closed", "false")
	q.Set("limit", strconv.Itoa(c.cfg.MarketCandidateLimit))
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gamma markets returned %s", resp.Status)
	}
	var raw []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	var out []types.MarketCandidate
	for _, item := range raw {
		candidate := types.MarketCandidate{
			EventSlug:     readString(item, "eventSlug", "event_slug", "event"),
			MarketSlug:    readString(item, "slug", "market_slug", "marketSlug"),
			Question:      readString(item, "question"),
			Outcomes:      readStringArray(item["outcomes"]),
			OutcomePrices: readNumberArray(item["outcomePrices"], item["outcome_prices"]),
			TokenIDs:      firstStringArray(item["clobTokenIds"], item["clob_token_ids"]),
			LiquidityUSD:  readFloat(item["liquidity"], item["liquidityNum"]),
			Resolution: types.Resolution{
				SourceURL:             readString(item, "resolutionSource", "resolution_source"),
				RuleExcerpt:           readString(item, "description", "rules"),
				TimeWindow:            readString(item, "endDate", "end_date"),
				UnresolvedAmbiguities: []string{},
			},
		}
		if candidate.MarketSlug != "" && len(candidate.TokenIDs) > 0 {
			out = append(out, candidate)
		}
	}
	return out, nil
}

func (c Client) ReadBook(ctx context.Context, tokenID string) (types.OrderBook, error) {
	u, err := url.Parse(c.cfg.CLOBBaseURL + "/book")
	if err != nil {
		return types.OrderBook{}, err
	}
	q := u.Query()
	q.Set("token_id", tokenID)
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return types.OrderBook{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return types.OrderBook{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return types.OrderBook{TokenID: tokenID}, nil
	}
	var raw map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return types.OrderBook{}, err
	}
	book := types.OrderBook{
		TokenID: tokenID,
		Bids:    readBookLevels(raw["bids"]),
		Asks:    readBookLevels(raw["asks"]),
	}
	if len(book.Bids) > 0 {
		v := book.Bids[0].Price
		book.BestBid = &v
	}
	if len(book.Asks) > 0 {
		v := book.Asks[0].Price
		book.BestAsk = &v
	}
	if v := readFloat(raw["min_order_size"], raw["minOrderSize"]); v > 0 {
		book.MinOrderSize = &v
	}
	return book, nil
}

func readString(record map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := record[key].(string); ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func readStringArray(value any) []string {
	switch v := value.(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s := strings.TrimSpace(fmt.Sprint(item)); s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		var parsed []string
		if json.Unmarshal([]byte(v), &parsed) == nil {
			return parsed
		}
		return splitCSV(v)
	default:
		return nil
	}
}

func firstStringArray(values ...any) []string {
	for _, value := range values {
		items := readStringArray(value)
		if len(items) > 0 {
			return items
		}
	}
	return nil
}

func readNumberArray(values ...any) []float64 {
	for _, value := range values {
		switch v := value.(type) {
		case []any:
			out := make([]float64, 0, len(v))
			for _, item := range v {
				out = append(out, readFloat(item))
			}
			return out
		case string:
			var parsed []float64
			if json.Unmarshal([]byte(v), &parsed) == nil {
				return parsed
			}
			strs := splitCSV(v)
			out := make([]float64, 0, len(strs))
			for _, s := range strs {
				out = append(out, readFloat(s))
			}
			return out
		}
	}
	return nil
}

func readBookLevels(value any) []types.BookLevel {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]types.BookLevel, 0, len(items))
	for _, item := range items {
		rec, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, types.BookLevel{Price: readFloat(rec["price"]), Size: readFloat(rec["size"])})
	}
	return out
}

func readFloat(values ...any) float64 {
	for _, value := range values {
		switch v := value.(type) {
		case float64:
			return v
		case int:
			return float64(v)
		case string:
			f, _ := strconv.ParseFloat(strings.TrimSpace(v), 64)
			return f
		}
	}
	return 0
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		s := strings.Trim(strings.TrimSpace(part), `"'[]`)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
