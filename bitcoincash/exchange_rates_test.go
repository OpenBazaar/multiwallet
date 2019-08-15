package bitcoincash

import (
	"bytes"
	"encoding/json"
	"io"
	gonet "net"
	"net/http"
	"testing"
	"time"
)

func setupBitcoinPriceFetcher() (b BitcoinCashPriceFetcher) {
	b = BitcoinCashPriceFetcher{
		cache: make(map[string]float64),
	}
	client := &http.Client{Transport: &http.Transport{Dial: gonet.Dial}, Timeout: time.Minute}
	b.providers = []*ExchangeRateProvider{
		{"https://ticker.openbazaar.org/api", b.cache, client, OpenBazaarDecoder{}},
	}
	return b
}

func TestFetchCurrentRates(t *testing.T) {
	b := setupBitcoinPriceFetcher()
	err := b.fetchCurrentRates()
	if err != nil {
		t.Error("Failed to fetch bitcoin exchange rates")
	}
}

func TestGetLatestRate(t *testing.T) {
	b := setupBitcoinPriceFetcher()
	price, err := b.GetLatestRate("USD")
	if err != nil || price == 0 {
		t.Error("Incorrect return at GetLatestRate (price, err)", price, err)
	}
	b.cache["USD"] = 650.00
	price, ok := b.cache["USD"]
	if !ok || price != 650 {
		t.Error("Failed to fetch exchange rates from cache")
	}
	price, err = b.GetLatestRate("USD")
	if err != nil || price == 650.00 {
		t.Error("Incorrect return at GetLatestRate (price, err)", price, err)
	}
}

func TestGetAllRates(t *testing.T) {
	b := setupBitcoinPriceFetcher()
	b.cache["USD"] = 650.00
	b.cache["EUR"] = 600.00
	priceMap, err := b.GetAllRates(true)
	if err != nil {
		t.Error(err)
	}
	usd, ok := priceMap["USD"]
	if !ok || usd != 650.00 {
		t.Error("Failed to fetch exchange rates from cache")
	}
	eur, ok := priceMap["EUR"]
	if !ok || eur != 600.00 {
		t.Error("Failed to fetch exchange rates from cache")
	}
}

func TestGetExchangeRate(t *testing.T) {
	b := setupBitcoinPriceFetcher()
	b.cache["USD"] = 650.00
	r, err := b.GetExchangeRate("USD")
	if err != nil {
		t.Error("Failed to fetch exchange rate")
	}
	if r != 650.00 {
		t.Error("Returned exchange rate incorrect")
	}
	r, err = b.GetExchangeRate("EUR")
	if r != 0 || err == nil {
		t.Error("Return erroneous exchange rate")
	}

	// Test that currency symbols are normalized correctly
	r, err = b.GetExchangeRate("usd")
	if err != nil {
		t.Error("Failed to fetch exchange rate")
	}
	if r != 650.00 {
		t.Error("Returned exchange rate incorrect")
	}
}

type req struct {
	io.Reader
}

func (r *req) Close() error {
	return nil
}

func TestDecodeOpenBazaar(t *testing.T) {
	cache := make(map[string]float64)
	openbazaarDecoder := OpenBazaarDecoder{}
	var dataMap interface{}

	response := `{
	  "AED": {
	    "ask": 2242.19,
	    "bid": 2236.61,
	    "last": 2239.99,
	    "timestamp": "Tue, 02 Aug 2016 00:20:45 -0000",
	    "volume_btc": 0.0,
	    "volume_percent": 0.0
	  },
	  "AFN": {
	    "ask": 41849.95,
	    "bid": 41745.86,
	    "last": 41808.85,
	    "timestamp": "Tue, 02 Aug 2016 00:20:45 -0000",
	    "volume_btc": 0.0,
	    "volume_percent": 0.0
	  },
	  "ALL": {
	    "ask": 74758.44,
	    "bid": 74572.49,
	    "last": 74685.02,
	    "timestamp": "Tue, 02 Aug 2016 00:20:45 -0000",
	    "volume_btc": 0.0,
	    "volume_percent": 0.0
	  },
	  "BCH": {
	    "ask":32.089016,
	    "bid":32.089016,
	    "last":32.089016,
	    "timestamp": "Tue, 02 Aug 2016 00:20:45 -0000"
	  },
	  "timestamp": "Tue, 02 Aug 2016 00:20:45 -0000"
	}`
	// Test valid response
	r := &req{bytes.NewReader([]byte(response))}
	decoder := json.NewDecoder(r)
	err := decoder.Decode(&dataMap)
	if err != nil {
		t.Error(err)
	}
	err = openbazaarDecoder.decode(dataMap, cache)
	if err != nil {
		t.Error(err)
	}
	// Make sure it saved to cache
	if len(cache) == 0 {
		t.Error("Failed to response to cache")
	}
	resp := `{"ZWL": {
	"ask": 196806.48,
	"bid": 196316.95,
	"timestamp": "Tue, 02 Aug 2016 00:20:45 -0000",
	"volume_btc": 0.0,
	"volume_percent": 0.0
	}}`

	// Test missing JSON element
	r = &req{bytes.NewReader([]byte(resp))}
	decoder = json.NewDecoder(r)
	err = decoder.Decode(&dataMap)
	if err != nil {
		t.Error(err)
	}
	err = openbazaarDecoder.decode(dataMap, cache)
	if err == nil {
		t.Error(err)
	}
	resp = `{
	"ask": 196806.48,
	"bid": 196316.95,
	"last": 196613.2,
	"timestamp": "Tue, 02 Aug 2016 00:20:45 -0000",
	"volume_btc": 0.0,
	"volume_percent": 0.0
	}`

	// Test invalid JSON
	r = &req{bytes.NewReader([]byte(resp))}
	decoder = json.NewDecoder(r)
	err = decoder.Decode(&dataMap)
	if err != nil {
		t.Error(err)
	}
	err = openbazaarDecoder.decode(dataMap, cache)
	if err == nil {
		t.Error(err)
	}

	// Test decode error
	r = &req{bytes.NewReader([]byte(""))}
	decoder = json.NewDecoder(r)
	decoder.Decode(&dataMap)
	err = openbazaarDecoder.decode(dataMap, cache)
	if err == nil {
		t.Error(err)
	}
}
