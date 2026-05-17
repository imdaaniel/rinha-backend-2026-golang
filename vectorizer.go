package main

import (
	"math"
	"time"
)

// Payload structures (subset)
type Payload struct {
	ID          string `json:"id"`
	Transaction struct {
		Amount       float64 `json:"amount"`
		Installments int     `json:"installments"`
		RequestedAt  string  `json:"requested_at"`
	} `json:"transaction"`
	Customer struct {
		AvgAmount      float64  `json:"avg_amount"`
		TxCount24h     int      `json:"tx_count_24h"`
		KnownMerchants []string `json:"known_merchants"`
	} `json:"customer"`
	Merchant struct {
		ID        string  `json:"id"`
		MCC       string  `json:"mcc"`
		AvgAmount float64 `json:"avg_amount"`
	} `json:"merchant"`
	Terminal struct {
		IsOnline    bool    `json:"is_online"`
		CardPresent bool    `json:"card_present"`
		KmFromHome  float64 `json:"km_from_home"`
	} `json:"terminal"`
	LastTransaction *struct {
		Timestamp     string  `json:"timestamp"`
		KmFromCurrent float64 `json:"km_from_current"`
	} `json:"last_transaction"`
}

type Reference struct {
	Vector []float32 `json:"vector"`
	Label  string    `json:"label"`
}

// normalization defaults
var (
	maxAmount                    = 10000.0
	maxInstallments              = 12.0
	amountVsAvgRatio             = 10.0
	maxMinutes                   = 1440.0
	maxKm                        = 1000.0
	maxTxCount24h                = 20.0
	maxMerchantAvgAmount         = 10000.0
	mccRiskDefault       float32 = 0.5
	mccRisk                      = map[string]float32{
		"5411": 0.15,
		"5812": 0.30,
		"5912": 0.20,
		"5944": 0.45,
		"7801": 0.80,
		"7802": 0.75,
		"7995": 0.85,
		"4511": 0.35,
		"5311": 0.25,
		"5999": 0.50,
	}
)

func clamp01(x float64) float32 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return 0.0
	}
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return float32(x)
}

func containsString(slice []string, v string) bool {
	for _, s := range slice {
		if s == v {
			return true
		}
	}
	return false
}

// Vectorize converts a Payload into a 14-dim vector per rules
func Vectorize(p *Payload) []float32 {
	v := make([]float32, 14)
	VectorizeTo(p, v)
	return v
}

// VectorizeTo writes the 14-dim vector to a pre-allocated slice to avoid allocations
func VectorizeTo(p *Payload, v []float32) {
	if len(v) < 14 {
		panic("vector slice must have at least 14 elements")
	}

	// dim 0: amount
	v[0] = clamp01(p.Transaction.Amount / maxAmount)

	// dim 1: installments
	v[1] = clamp01(float64(p.Transaction.Installments) / maxInstallments)

	// dim 2: amount_vs_avg
	avg := p.Customer.AvgAmount
	ratio := 0.0
	if avg > 0 {
		ratio = (p.Transaction.Amount / avg) / amountVsAvgRatio
	}
	v[2] = clamp01(ratio)

	// parse requested_at
	t, err := time.Parse(time.RFC3339, p.Transaction.RequestedAt)
	if err != nil {
		t = time.Now().UTC()
	}

	// dim 3: hour_of_day
	v[3] = clamp01(float64(t.Hour()) / 23.0)

	// dim 4: day_of_week (Mon=0..Sun=6) note: Go Weekday: Sunday=0
	// convert to Mon=0..Sun=6
	dow := int(t.Weekday())
	if dow == 0 {
		dow = 6
	} else {
		dow = dow - 1
	}
	v[4] = clamp01(float64(dow) / 6.0)

	// dim 5 & 6: minutes_since_last_tx, km_from_last_tx or -1
	if p.LastTransaction == nil {
		v[5] = -1
		v[6] = -1
	} else {
		lt, err := time.Parse(time.RFC3339, p.LastTransaction.Timestamp)
		if err != nil {
			lt = t
		}
		minutes := t.Sub(lt).Minutes()
		v[5] = clamp01(minutes / maxMinutes)
		v[6] = clamp01(p.LastTransaction.KmFromCurrent / maxKm)
	}

	// dim 7: km_from_home
	v[7] = clamp01(p.Terminal.KmFromHome / maxKm)

	// dim 8: tx_count_24h
	v[8] = clamp01(float64(p.Customer.TxCount24h) / maxTxCount24h)

	// dim 9: is_online
	if p.Terminal.IsOnline {
		v[9] = 1
	} else {
		v[9] = 0
	}

	// dim 10: card_present
	if p.Terminal.CardPresent {
		v[10] = 1
	} else {
		v[10] = 0
	}

	// dim 11: unknown_merchant
	if containsString(p.Customer.KnownMerchants, p.Merchant.ID) {
		v[11] = 0
	} else {
		v[11] = 1
	}

	// dim 12: mcc_risk
	if val, ok := mccRisk[p.Merchant.MCC]; ok {
		v[12] = val
	} else {
		v[12] = mccRiskDefault
	}

	// dim 13: merchant_avg_amount
	v[13] = clamp01(p.Merchant.AvgAmount / maxMerchantAvgAmount)
}

// BruteForceKNNScore computes fraction of 'fraud' labels among k nearest
func BruteForceKNNScore(q []float32, refs []Reference, k int) float32 {
	if len(refs) == 0 {
		return 0.0
	}
	type pair struct {
		idx  int
		dist float32
	}
	ps := make([]pair, 0, len(refs))
	for i, r := range refs {
		d := distSq(q, r.Vector)
		ps = append(ps, pair{i, d})
	}
	// partial selection: simple sort for prototype
	// using naive sort
	for i := 0; i < len(ps); i++ {
		for j := i + 1; j < len(ps); j++ {
			if ps[j].dist < ps[i].dist {
				ps[i], ps[j] = ps[j], ps[i]
			}
		}
	}
	take := k
	if take > len(ps) {
		take = len(ps)
	}
	frauds := 0
	for i := 0; i < take; i++ {
		if refs[ps[i].idx].Label == "fraud" {
			frauds++
		}
	}
	return float32(frauds) / float32(take)
}

func distSq(a, b []float32) float32 {
	la := len(a)
	lb := len(b)
	n := la
	if lb < n {
		n = lb
	}
	
	// Loop unrolling for better performance
	var s float32
	i := 0
	// Process 4 elements at a time
	for i+4 <= n {
		dx0 := a[i] - b[i]
		dx1 := a[i+1] - b[i+1]
		dx2 := a[i+2] - b[i+2]
		dx3 := a[i+3] - b[i+3]
		s += dx0*dx0 + dx1*dx1 + dx2*dx2 + dx3*dx3
		i += 4
	}
	// Process remaining elements
	for i < n {
		dx := a[i] - b[i]
		s += dx * dx
		i++
	}
	
	if la != lb {
		var rem float32
		if la > lb {
			for i := lb; i < la; i++ {
				rem += a[i] * a[i]
			}
		} else {
			for i := la; i < lb; i++ {
				rem += b[i] * b[i]
			}
		}
		s += rem
	}
	if math.IsNaN(float64(s)) || math.IsInf(float64(s), 0) {
		return 1e9
	}
	return s
}
