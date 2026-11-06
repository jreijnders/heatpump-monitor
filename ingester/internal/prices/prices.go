// Package prices fetches easyEnergy dynamic energy prices and stores them in energy_prices
// (db/schema/060_energy_prices.sql, ADR-0004).
package prices

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DefaultBaseURL is easyEnergy's public price-graph API (no authentication).
const DefaultBaseURL = "https://price-graph.mijn.easyenergy.com/api/prices"

// Commodity names as stored in energy_prices.commodity and sent as the API's type parameter.
const (
	Electricity = "electricity"
	Gas         = "gas"
)

// granularity per commodity: electricity at the day-ahead market's quarter-hour resolution,
// gas per day (the only resolution easyEnergy publishes).
var granularity = map[string]string{Electricity: "quarter", Gas: "day"}

// Price is one published price interval [Starts, Ends). Amounts are EUR per Unit.
type Price struct {
	Commodity    string
	Starts, Ends time.Time
	Unit         string
	MarketExVAT  float64 // day-ahead market price
	MarketIncVAT float64
	EnergyTax    float64 // incl. VAT
	PurchaseFee  float64 // easyEnergy inkoopvergoeding, incl. VAT
	Invoice      float64 // all-in: MarketIncVAT + EnergyTax + PurchaseFee
}

type apiPrice struct {
	From          string  `json:"from"`
	Until         string  `json:"until"`
	Price         float64 `json:"price"`
	Unit          string  `json:"unit"`
	PriceIncVat   float64 `json:"priceIncVat"`
	EnergyTax     float64 `json:"energyTax"`
	PurchasePrice float64 `json:"purchasePrice"`
	InvoicePrice  float64 `json:"invoicePrice"`
}

// apiTimeLayout: easyEnergy sends wall-clock times without offset, e.g. 2026-11-05T00:15:00.0000000.
const apiTimeLayout = "2006-01-02T15:04:05.9999999"

// Parse decodes a price-graph response. Times are wall-clock in loc (Europe/Amsterdam). On the
// autumn DST day the repeated hour is published once; intervals that collapse to zero or negative
// length are dropped.
func Parse(commodity string, body []byte, loc *time.Location) ([]Price, error) {
	var resp struct {
		Prices []apiPrice `json:"prices"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode %s prices: %w", commodity, err)
	}
	out := make([]Price, 0, len(resp.Prices))
	for _, p := range resp.Prices {
		starts, err := time.ParseInLocation(apiTimeLayout, p.From, loc)
		if err != nil {
			return nil, fmt.Errorf("%s price from %q: %w", commodity, p.From, err)
		}
		ends, err := time.ParseInLocation(apiTimeLayout, p.Until, loc)
		if err != nil {
			return nil, fmt.Errorf("%s price until %q: %w", commodity, p.Until, err)
		}
		if !ends.After(starts) {
			continue
		}
		out = append(out, Price{
			Commodity:    commodity,
			Starts:       starts,
			Ends:         ends,
			Unit:         p.Unit,
			MarketExVAT:  p.Price,
			MarketIncVAT: p.PriceIncVat,
			EnergyTax:    p.EnergyTax,
			PurchaseFee:  p.PurchasePrice,
			Invoice:      p.InvoicePrice,
		})
	}
	return out, nil
}

// Client fetches prices from the easyEnergy API.
type Client struct {
	BaseURL string
	HTTP    *http.Client
	Loc     *time.Location
}

// Fetch returns the commodity's prices for local days [from, to) (dates in c.Loc).
func (c Client) Fetch(ctx context.Context, commodity string, from, to time.Time) ([]Price, error) {
	g, ok := granularity[commodity]
	if !ok {
		return nil, fmt.Errorf("unknown commodity %q", commodity)
	}
	q := url.Values{
		"start":       {from.In(c.Loc).Format(time.DateOnly)},
		"end":         {to.In(c.Loc).Format(time.DateOnly)},
		"type":        {commodity},
		"granularity": {g},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s prices: %w", commodity, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, fmt.Errorf("read %s prices: %w", commodity, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s prices: HTTP %d", commodity, resp.StatusCode)
	}
	return Parse(commodity, body, c.Loc)
}

const upsertSQL = `
INSERT INTO energy_prices (commodity, starts, ends, unit, market_price_ex_vat, market_price_inc_vat,
                           energy_tax, purchase_fee, invoice_price, fetched_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())
ON CONFLICT (commodity, starts) DO UPDATE SET
    ends = EXCLUDED.ends, unit = EXCLUDED.unit,
    market_price_ex_vat = EXCLUDED.market_price_ex_vat,
    market_price_inc_vat = EXCLUDED.market_price_inc_vat,
    energy_tax = EXCLUDED.energy_tax, purchase_fee = EXCLUDED.purchase_fee,
    invoice_price = EXCLUDED.invoice_price, fetched_at = EXCLUDED.fetched_at`

// Upsert writes ps to energy_prices, replacing earlier versions of the same interval
// (easyEnergy may revise tax or fee components).
func Upsert(ctx context.Context, pool *pgxpool.Pool, ps []Price) error {
	if len(ps) == 0 {
		return nil
	}
	b := &pgx.Batch{}
	for _, p := range ps {
		b.Queue(upsertSQL, p.Commodity, p.Starts, p.Ends, p.Unit, p.MarketExVAT, p.MarketIncVAT,
			p.EnergyTax, p.PurchaseFee, p.Invoice)
	}
	if err := pool.SendBatch(ctx, b).Close(); err != nil {
		return fmt.Errorf("upsert %d prices: %w", len(ps), err)
	}
	return nil
}
