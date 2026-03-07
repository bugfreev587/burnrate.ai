package provider

import (
	"sync"
)

// ModelPricing holds the cost per 1M tokens (input/output) for a model.
type ModelPricing struct {
	InputPer1M  float64 // cost per 1M input tokens
	OutputPer1M float64 // cost per 1M output tokens
}

// CostResult holds the computed cost for a single request.
type CostResult struct {
	InputCost  float64
	OutputCost float64
	TotalCost  float64
	Model      string
	Provider   string
}

// PricingTable maintains a thread-safe map of model → pricing.
// Supports dynamic updates as providers change prices.
type PricingTable struct {
	mu     sync.RWMutex
	prices map[string]ModelPricing
}

// defaultPrices is the built-in pricing table (per 1M tokens, USD).
var defaultPrices = map[string]ModelPricing{
	// OpenAI
	"gpt-4o":            {InputPer1M: 2.50, OutputPer1M: 10.00},
	"gpt-4o-mini":       {InputPer1M: 0.15, OutputPer1M: 0.60},
	"gpt-4-turbo":       {InputPer1M: 10.00, OutputPer1M: 30.00},
	"gpt-4":             {InputPer1M: 30.00, OutputPer1M: 60.00},
	"o1":                {InputPer1M: 15.00, OutputPer1M: 60.00},
	"o1-mini":           {InputPer1M: 3.00, OutputPer1M: 12.00},
	"o3":                {InputPer1M: 10.00, OutputPer1M: 40.00},
	"o3-mini":           {InputPer1M: 1.10, OutputPer1M: 4.40},
	"o4-mini":           {InputPer1M: 1.10, OutputPer1M: 4.40},

	// Anthropic
	"claude-opus-4-20250514":        {InputPer1M: 15.00, OutputPer1M: 75.00},
	"claude-sonnet-4-20250514":      {InputPer1M: 3.00, OutputPer1M: 15.00},
	"claude-haiku-4-5-20251001":     {InputPer1M: 0.80, OutputPer1M: 4.00},
	"claude-3-5-sonnet-20241022":    {InputPer1M: 3.00, OutputPer1M: 15.00},
	"claude-3-5-haiku-20241022":     {InputPer1M: 0.80, OutputPer1M: 4.00},

	// DeepSeek
	"deepseek-chat":     {InputPer1M: 0.27, OutputPer1M: 1.10},
	"deepseek-reasoner": {InputPer1M: 0.55, OutputPer1M: 2.19},

	// Mistral
	"mistral-large-latest": {InputPer1M: 2.00, OutputPer1M: 6.00},
	"mistral-small-latest": {InputPer1M: 0.10, OutputPer1M: 0.30},
	"codestral-latest":     {InputPer1M: 0.30, OutputPer1M: 0.90},

	// Gemini
	"gemini-2.0-flash":    {InputPer1M: 0.10, OutputPer1M: 0.40},
	"gemini-2.0-flash-lite": {InputPer1M: 0.02, OutputPer1M: 0.10},
	"gemini-1.5-pro":      {InputPer1M: 1.25, OutputPer1M: 5.00},
	"gemini-1.5-flash":    {InputPer1M: 0.075, OutputPer1M: 0.30},
}

func NewPricingTable() *PricingTable {
	pt := &PricingTable{
		prices: make(map[string]ModelPricing, len(defaultPrices)),
	}
	for k, v := range defaultPrices {
		pt.prices[k] = v
	}
	return pt
}

// Get returns the pricing for a model. Returns zero pricing if model is unknown.
func (pt *PricingTable) Get(model string) (ModelPricing, bool) {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	p, ok := pt.prices[model]
	return p, ok
}

// Set updates the pricing for a model (for dynamic price changes).
func (pt *PricingTable) Set(model string, pricing ModelPricing) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	pt.prices[model] = pricing
}

// CalculateCost computes the cost of a request based on token usage.
func (pt *PricingTable) CalculateCost(model, providerName string, usage *Usage) *CostResult {
	if usage == nil {
		return &CostResult{Model: model, Provider: providerName}
	}

	pricing, ok := pt.Get(model)
	if !ok {
		return &CostResult{Model: model, Provider: providerName}
	}

	inputCost := float64(usage.PromptTokens) / 1_000_000 * pricing.InputPer1M
	outputCost := float64(usage.CompletionTokens) / 1_000_000 * pricing.OutputPer1M

	return &CostResult{
		InputCost:  inputCost,
		OutputCost: outputCost,
		TotalCost:  inputCost + outputCost,
		Model:      model,
		Provider:   providerName,
	}
}
