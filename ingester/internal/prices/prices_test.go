package prices

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func amsterdam(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestParseWinterQuarter(t *testing.T) {
	body := []byte(`{"prices":[{"from":"2026-11-05T00:15:00.0000000","until":"2026-11-05T00:30:00.0000000",
		"price":0.19867,"unit":"kWh","priceIncVat":0.24039,"energyTax":0.11085,"purchasePrice":0.02178,
		"invoicePrice":0.37302,"granularity":"quarter"}]}`)
	ps, err := Parse(Electricity, body, amsterdam(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 {
		t.Fatalf("got %d prices, want 1", len(ps))
	}
	p := ps[0]
	if want := time.Date(2026, 11, 4, 23, 15, 0, 0, time.UTC); !p.Starts.Equal(want) {
		t.Errorf("Starts = %v, want %v (CET = UTC+1)", p.Starts.UTC(), want)
	}
	if got := p.Ends.Sub(p.Starts); got != 15*time.Minute {
		t.Errorf("length = %v, want 15m", got)
	}
	if p.Commodity != Electricity || p.Unit != "kWh" || p.MarketExVAT != 0.19867 || p.MarketIncVAT != 0.24039 ||
		p.EnergyTax != 0.11085 || p.PurchaseFee != 0.02178 || p.Invoice != 0.37302 {
		t.Errorf("fields = %+v", p)
	}
}

func TestParseSummerIsCEST(t *testing.T) {
	body := []byte(`{"prices":[{"from":"2026-06-01T13:00:00.0000000","until":"2026-06-01T13:15:00.0000000",
		"price":0,"unit":"kWh","priceIncVat":0,"energyTax":0.11085,"purchasePrice":0.02178,"invoicePrice":0.13263}]}`)
	ps, err := Parse(Electricity, body, amsterdam(t))
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 6, 1, 11, 0, 0, 0, time.UTC); !ps[0].Starts.Equal(want) {
		t.Errorf("Starts = %v, want %v (CEST = UTC+2)", ps[0].Starts.UTC(), want)
	}
}

func TestParseGasDay(t *testing.T) {
	body := []byte(`{"prices":[{"from":"2026-11-05T00:00:00.0000000","until":"2026-11-06T00:00:00.0000000",
		"price":0.87,"unit":"m3","priceIncVat":1.0527,"energyTax":0.7268,"purchasePrice":0.079,"invoicePrice":1.8585}]}`)
	ps, err := Parse(Gas, body, amsterdam(t))
	if err != nil {
		t.Fatal(err)
	}
	if got := ps[0].Ends.Sub(ps[0].Starts); got != 24*time.Hour {
		t.Errorf("length = %v, want 24h", got)
	}
	if ps[0].Invoice != 1.8585 || ps[0].Unit != "m3" {
		t.Errorf("fields = %+v", ps[0])
	}
}

func TestParseRejectsBadTime(t *testing.T) {
	_, err := Parse(Gas, []byte(`{"prices":[{"from":"yesterday","until":"2026-11-06T00:00:00"}]}`), amsterdam(t))
	if err == nil {
		t.Fatal("want error")
	}
}

func TestFetchSendsLocalDatesAndGranularity(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.RawQuery
		w.Write([]byte(`{"prices":[]}`))
	}))
	defer srv.Close()
	loc := amsterdam(t)
	c := Client{BaseURL: srv.URL, HTTP: srv.Client(), Loc: loc}
	from := time.Date(2026, 11, 4, 23, 30, 0, 0, time.UTC) // 2026-11-05 00:30 local
	if _, err := c.Fetch(context.Background(), Electricity, from, from.Add(48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	want := "end=2026-11-07&granularity=quarter&start=2026-11-05&type=electricity"
	if got != want {
		t.Errorf("query = %q, want %q", got, want)
	}
}

func TestFetchHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer srv.Close()
	c := Client{BaseURL: srv.URL, HTTP: srv.Client(), Loc: amsterdam(t)}
	if _, err := c.Fetch(context.Background(), Gas, time.Now(), time.Now()); err == nil {
		t.Fatal("want error")
	}
}
