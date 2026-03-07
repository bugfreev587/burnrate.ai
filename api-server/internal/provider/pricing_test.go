package provider

import (
	"math"
	"testing"
)

func TestPricingTable_Get(t *testing.T) {
	pt := NewPricingTable()

	tests := []struct {
		model    string
		wantOK   bool
		wantIn   float64
		wantOut  float64
	}{
		{"gpt-4o", true, 2.50, 10.00},
		{"gpt-4o-mini", true, 0.15, 0.60},
		{"claude-sonnet-4-20250514", true, 3.00, 15.00},
		{"deepseek-chat", true, 0.27, 1.10},
		{"mistral-large-latest", true, 2.00, 6.00},
		{"gemini-2.0-flash", true, 0.10, 0.40},
		{"nonexistent-model", false, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			p, ok := pt.Get(tt.model)
			if ok != tt.wantOK {
				t.Errorf("Get(%s): expected ok=%v, got %v", tt.model, tt.wantOK, ok)
			}
			if ok {
				if p.InputPer1M != tt.wantIn {
					t.Errorf("InputPer1M: expected %v, got %v", tt.wantIn, p.InputPer1M)
				}
				if p.OutputPer1M != tt.wantOut {
					t.Errorf("OutputPer1M: expected %v, got %v", tt.wantOut, p.OutputPer1M)
				}
			}
		})
	}
}

func TestPricingTable_Set(t *testing.T) {
	pt := NewPricingTable()

	pt.Set("new-model", ModelPricing{InputPer1M: 1.0, OutputPer1M: 5.0})
	p, ok := pt.Get("new-model")
	if !ok {
		t.Fatal("expected to find new-model")
	}
	if p.InputPer1M != 1.0 {
		t.Errorf("expected 1.0, got %v", p.InputPer1M)
	}

	// Override existing
	pt.Set("gpt-4o", ModelPricing{InputPer1M: 99.0, OutputPer1M: 99.0})
	p, _ = pt.Get("gpt-4o")
	if p.InputPer1M != 99.0 {
		t.Errorf("expected override to 99.0, got %v", p.InputPer1M)
	}
}

func TestPricingTable_CalculateCost(t *testing.T) {
	pt := NewPricingTable()

	tests := []struct {
		name      string
		model     string
		usage     *Usage
		wantTotal float64
	}{
		{
			name:  "gpt-4o: 1000 input + 500 output",
			model: "gpt-4o",
			usage: &Usage{PromptTokens: 1000, CompletionTokens: 500, TotalTokens: 1500},
			// 1000/1M * 2.50 = 0.0025, 500/1M * 10.00 = 0.005 → total 0.0075
			wantTotal: 0.0075,
		},
		{
			name:  "claude-sonnet: 10000 input + 2000 output",
			model: "claude-sonnet-4-20250514",
			usage: &Usage{PromptTokens: 10000, CompletionTokens: 2000, TotalTokens: 12000},
			// 10000/1M * 3.00 = 0.03, 2000/1M * 15.00 = 0.03 → total 0.06
			wantTotal: 0.06,
		},
		{
			name:  "deepseek-chat: 5000 input + 1000 output",
			model: "deepseek-chat",
			usage: &Usage{PromptTokens: 5000, CompletionTokens: 1000, TotalTokens: 6000},
			// 5000/1M * 0.27 = 0.00135, 1000/1M * 1.10 = 0.0011 → total 0.00245
			wantTotal: 0.00245,
		},
		{
			name:      "nil usage",
			model:     "gpt-4o",
			usage:     nil,
			wantTotal: 0,
		},
		{
			name:      "unknown model",
			model:     "nonexistent",
			usage:     &Usage{PromptTokens: 1000, CompletionTokens: 500},
			wantTotal: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pt.CalculateCost(tt.model, "test", tt.usage)
			if math.Abs(result.TotalCost-tt.wantTotal) > 1e-9 {
				t.Errorf("expected total cost %v, got %v", tt.wantTotal, result.TotalCost)
			}
		})
	}
}

func TestPricingTable_CalculateCost_Components(t *testing.T) {
	pt := NewPricingTable()

	usage := &Usage{PromptTokens: 1_000_000, CompletionTokens: 1_000_000, TotalTokens: 2_000_000}
	result := pt.CalculateCost("gpt-4o", "openai", usage)

	if math.Abs(result.InputCost-2.50) > 1e-9 {
		t.Errorf("expected input cost 2.50, got %v", result.InputCost)
	}
	if math.Abs(result.OutputCost-10.00) > 1e-9 {
		t.Errorf("expected output cost 10.00, got %v", result.OutputCost)
	}
	if result.Model != "gpt-4o" {
		t.Errorf("expected model gpt-4o, got %s", result.Model)
	}
	if result.Provider != "openai" {
		t.Errorf("expected provider openai, got %s", result.Provider)
	}
}
