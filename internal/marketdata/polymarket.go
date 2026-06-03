package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
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

var eventSortDimensions = []string{"volume24hr", "liquidity", "startDate", "competitive"}

var pricePredictionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)price.*(?:above|below|hit|reach|dip|pump)`),
	regexp.MustCompile(`(?i)(?:BTC|ETH|SOL|DOGE).*\$[\d,]+`),
	regexp.MustCompile(`(?i)Bitcoin.*\$|Ethereum.*\$`),
	regexp.MustCompile(`(?i)up or down`),
	regexp.MustCompile(`(?i)\d{1,2}:\d{2}\s*(AM|PM)\s*(?:-|ET)`),
}

var typeWeights = map[string]float64{
	"politics":       1.5,
	"geopolitics":    1.5,
	"foreign-policy": 1.5,
	"tech":           1.5,
	"ai":             1.5,
	"economics":      1.2,
	"finance":        1.2,
	"sports":         1.0,
	"esports":        1.0,
	"culture":        1.0,
	"crypto":         0.3,
	"commodities":    0.3,
	"weather":        0.5,
	"other":          0.8,
}

var typeAliases = []struct {
	pattern   string
	canonical string
}{
	{"politic", "politics"},
	{"trump", "politics"},
	{"election", "politics"},
	{"geopolitic", "geopolitics"},
	{"war", "geopolitics"},
	{"conflict", "geopolitics"},
	{"foreign-policy", "foreign-policy"},
	{"diplomac", "foreign-policy"},
	{"tech", "tech"},
	{"ai", "ai"},
	{"econ", "economics"},
	{"fed", "economics"},
	{"inflation", "economics"},
	{"gdp", "economics"},
	{"finance", "finance"},
	{"stock", "finance"},
	{"sport", "sports"},
	{"nba", "sports"},
	{"nfl", "sports"},
	{"mlb", "sports"},
	{"soccer", "sports"},
	{"football", "sports"},
	{"esport", "esports"},
	{"culture", "culture"},
	{"entertain", "culture"},
	{"movie", "culture"},
	{"music", "culture"},
	{"oscar", "culture"},
	{"crypto", "crypto"},
	{"bitcoin", "crypto"},
	{"ethereum", "crypto"},
	{"defi", "crypto"},
	{"weather", "weather"},
	{"climate", "weather"},
	{"hurricane", "weather"},
	{"commodit", "commodities"},
}

func New(cfg config.Config) Client {
	return Client{cfg: cfg, http: &http.Client{Timeout: 20 * time.Second}}
}

func (c Client) FetchCandidates(ctx context.Context) ([]types.MarketCandidate, error) {
	candidates, _, err := c.FetchCandidateSet(ctx)
	return candidates, err
}

func (c Client) FetchCandidateSet(ctx context.Context) ([]types.MarketCandidate, types.DiscoveryReport, error) {
	pageSize := c.cfg.EventsPerPage
	if pageSize <= 0 || pageSize > 50 {
		pageSize = 50
	}
	pages := c.cfg.EventPagesPerSort
	seen := map[string]bool{}
	var out []types.MarketCandidate
	report := types.DiscoveryReport{
		SortDimensions:   []types.DiscoverySortReport{},
		FilteredByReason: map[string]int{},
	}
	acceptedEvents := map[string]bool{}
	for _, dimension := range eventSortDimensions {
		sortReport := types.DiscoverySortReport{Order: dimension}
		for page := 0; pages <= 0 || page < pages; page++ {
			rawEvents, err := c.fetchEventsPage(ctx, pageSize, page*pageSize, dimension)
			if err != nil {
				report.AcceptedMarketCount = len(out)
				report.AcceptedEventCount = len(acceptedEvents)
				report.SortDimensions = append(report.SortDimensions, sortReport)
				return out, report, err
			}
			sortReport.Pages++
			sortReport.Events += len(rawEvents)
			report.RawEventCount += len(rawEvents)
			for _, event := range rawEvents {
				for _, market := range extractEventMarkets(event) {
					report.RawMarketCount++
					candidate := parseEventMarketCandidate(event, market)
					if include, notes := candidatePassesFilters(candidate, c.cfg); !include {
						candidate.FilterNotes = notes
						report.FilteredMarketCount++
						for _, note := range notes {
							report.FilteredByReason[note]++
						}
						continue
					}
					candidate.Score = candidateScore(candidate)
					key := candidate.MarketSlug
					if key == "" {
						key = strings.Join(candidate.TokenIDs, ",")
					}
					if seen[key] {
						report.DuplicateMarketCount++
						continue
					}
					seen[key] = true
					acceptedEvents[candidate.EventSlug] = true
					out = append(out, candidate)
				}
			}
			if len(rawEvents) < pageSize {
				sortReport.ShortPage = true
				break
			}
		}
		report.SortDimensions = append(report.SortDimensions, sortReport)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Score > out[j].Score
	})
	report.AcceptedMarketCount = len(out)
	report.AcceptedEventCount = len(acceptedEvents)
	return out, report, nil
}

func (c Client) fetchEventsPage(ctx context.Context, limit, offset int, order string) ([]map[string]any, error) {
	u, err := url.Parse(c.cfg.GammaBaseURL + "/events")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("active", "true")
	q.Set("closed", "false")
	q.Set("limit", strconv.Itoa(limit))
	q.Set("offset", strconv.Itoa(offset))
	q.Set("order", order)
	q.Set("ascending", "false")
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
		return nil, fmt.Errorf("gamma events returned %s", resp.Status)
	}
	var raw []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func extractEventMarkets(event map[string]any) []map[string]any {
	items, ok := event["markets"].([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		rec, ok := item.(map[string]any)
		if ok {
			out = append(out, rec)
		}
	}
	return out
}

func parseEventMarketCandidate(event, market map[string]any) types.MarketCandidate {
	categorySlug, categoryLabel := primaryCategory(event)
	tags := readTags(event["tags"])
	return types.MarketCandidate{
		EventSlug:       readString(event, "slug", "eventSlug", "event_slug"),
		EventTitle:      readString(event, "title"),
		MarketSlug:      readString(market, "slug", "market_slug", "marketSlug"),
		Question:        readString(market, "question"),
		Outcomes:        readStringArray(market["outcomes"]),
		OutcomePrices:   readNumberArray(market["outcomePrices"], market["outcome_prices"]),
		TokenIDs:        firstStringArray(market["clobTokenIds"], market["clob_token_ids"]),
		LiquidityUSD:    readFloat(market["liquidityNum"], market["liquidity"], event["liquidity"]),
		Volume24hUSD:    readFloat(market["volume24hr"], event["volume24hr"]),
		BestBid:         readFloat(market["bestBid"]),
		BestAsk:         readFloat(market["bestAsk"]),
		Spread:          readFloat(market["spread"]),
		EnableOrderBook: readBool(market["enableOrderBook"]),
		CategorySlug:    categorySlug,
		CategoryLabel:   categoryLabel,
		Tags:            tags,
		Resolution: types.Resolution{
			SourceURL:             readString(market, "resolutionSource", "resolution_source", "resolvedBy"),
			RuleExcerpt:           readString(market, "description", "rules"),
			TimeWindow:            readString(market, "endDate", "end_date", "endDateIso"),
			UnresolvedAmbiguities: []string{},
		},
	}
}

func candidatePassesFilters(candidate types.MarketCandidate, cfg config.Config) (bool, []string) {
	var notes []string
	if !candidate.EnableOrderBook {
		notes = append(notes, "order_book_disabled")
	}
	if len(candidate.TokenIDs) == 0 {
		notes = append(notes, "missing_clob_token_ids")
	}
	if candidate.LiquidityUSD < cfg.MinMarketLiquidityUSD {
		notes = append(notes, "below_min_liquidity")
	}
	if len(candidate.OutcomePrices) == 0 {
		notes = append(notes, "missing_outcome_prices")
	} else {
		maxPrice := maxFloat(candidate.OutcomePrices)
		minPrice := minFloat(candidate.OutcomePrices)
		if maxPrice > 0.95 || minPrice < 0.05 {
			notes = append(notes, "extreme_price")
		}
		if isCoinFlip(candidate.OutcomePrices) {
			notes = append(notes, "coin_flip_prices")
		}
	}
	if isPricePredictionQuestion(candidate.Question) {
		notes = append(notes, "price_prediction_keyword")
	}
	return len(notes) == 0, notes
}

func candidateScore(candidate types.MarketCandidate) float64 {
	liquidityScore := math.Log10(candidate.LiquidityUSD+1) * math.Log10(candidate.Volume24hUSD+1)
	return liquidityScore * lookupTypeWeight(candidate.CategorySlug)
}

func lookupTypeWeight(categorySlug string) float64 {
	lower := strings.TrimSpace(strings.ToLower(categorySlug))
	if lower == "" {
		return typeWeights["other"]
	}
	if value, ok := typeWeights[lower]; ok {
		return value
	}
	for _, alias := range typeAliases {
		if strings.Contains(lower, alias.pattern) {
			if value, ok := typeWeights[alias.canonical]; ok {
				return value
			}
		}
	}
	return typeWeights["other"]
}

func isPricePredictionQuestion(question string) bool {
	for _, pattern := range pricePredictionPatterns {
		if pattern.MatchString(question) {
			return true
		}
	}
	return false
}

func isCoinFlip(prices []float64) bool {
	if len(prices) == 0 {
		return false
	}
	for _, price := range prices {
		if price < 0.48 || price > 0.52 {
			return false
		}
	}
	return true
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
	if v, ok := bestBid(book.Bids); ok {
		book.BestBid = &v
	}
	if v, ok := bestAsk(book.Asks); ok {
		book.BestAsk = &v
	}
	if v := readFloat(raw["min_order_size"], raw["minOrderSize"]); v > 0 {
		book.MinOrderSize = &v
	}
	return book, nil
}

func bestBid(levels []types.BookLevel) (float64, bool) {
	best := -math.MaxFloat64
	for _, level := range levels {
		if level.Price > best {
			best = level.Price
		}
	}
	return best, len(levels) > 0
}

func bestAsk(levels []types.BookLevel) (float64, bool) {
	best := math.MaxFloat64
	for _, level := range levels {
		if level.Price < best {
			best = level.Price
		}
	}
	return best, len(levels) > 0
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

func readTags(value any) []types.Tag {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]types.Tag, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		rec, ok := item.(map[string]any)
		if !ok {
			continue
		}
		slug := readString(rec, "slug")
		label := readString(rec, "label", "name")
		if slug == "" {
			slug = slugify(label)
		}
		if slug == "" || seen[slug] {
			continue
		}
		seen[slug] = true
		if label == "" {
			label = slug
		}
		out = append(out, types.Tag{Slug: slug, Label: label})
	}
	return out
}

func primaryCategory(event map[string]any) (string, string) {
	category := event["category"]
	if rec, ok := category.(map[string]any); ok {
		slug := readString(rec, "slug")
		label := readString(rec, "label", "name")
		if slug == "" {
			slug = slugify(label)
		}
		if slug != "" || label != "" {
			if label == "" {
				label = slug
			}
			return slug, label
		}
	}
	if s, ok := category.(string); ok && strings.TrimSpace(s) != "" {
		label := strings.TrimSpace(s)
		return slugify(label), label
	}
	tags := readTags(event["tags"])
	if len(tags) > 0 {
		return tags[0].Slug, tags[0].Label
	}
	return "uncategorized", "Uncategorized"
}

func slugify(value string) string {
	lower := strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, ch := range lower {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') {
			b.WriteRune(ch)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func minFloat(values []float64) float64 {
	min := math.MaxFloat64
	for _, value := range values {
		if value < min {
			min = value
		}
	}
	return min
}

func maxFloat(values []float64) float64 {
	max := -math.MaxFloat64
	for _, value := range values {
		if value > max {
			max = value
		}
	}
	return max
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

func readBool(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		parsed, _ := strconv.ParseBool(strings.TrimSpace(v))
		return parsed
	default:
		return false
	}
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
