// Package llm holds the provider implementations openshin uses for the parts of
// the pipeline that need a model: summarising, embedding, and extracting.
// The agent itself is the brain; openshin only calls the model for these narrow,
// well-defined tasks.
package llm

import (
	"context"
	"fmt"
	"os"
	"time"

	"google.golang.org/genai"
)

const (
	defaultGeminiModel   = "gemini-2.5-flash"
	defaultGeminiTimeout = 60 * time.Second
)

// summarizePrompt asks the model to compress a stretch of conversation into a
// short, self-contained summary for the rolling short-term window.
const summarizePrompt = `You compress a stretch of chat messages into a short summary for a rolling memory window.

Rules:
- Keep only what a future turn would need to know: decisions, facts, and open threads.
- Write plain prose, as short as possible. Output only the summary.`

// GeminiConfig carries the settings for the Gemini provider. Zero values fall
// back to defaults.
type GeminiConfig struct {
	APIKey  string
	Model   string
	Timeout time.Duration
}

// GeminiConfigFromEnv reads the provider settings from the environment. Only the
// API key is required.
func GeminiConfigFromEnv() (GeminiConfig, error) {
	cfg := GeminiConfig{
		APIKey: os.Getenv("GEMINI_API_KEY"),
		Model:  os.Getenv("GEMINI_MODEL"),
	}
	if cfg.APIKey == "" {
		return GeminiConfig{}, fmt.Errorf("gemini: GEMINI_API_KEY is not set")
	}
	return cfg, nil
}

// Gemini is the Gemini Developer API provider.
type Gemini struct {
	client  *genai.Client
	model   string
	timeout time.Duration
}

// NewGemini builds a Gemini provider.
func NewGemini(ctx context.Context, cfg GeminiConfig) (*Gemini, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("gemini: api key is required")
	}
	if cfg.Model == "" {
		cfg.Model = defaultGeminiModel
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultGeminiTimeout
	}

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  cfg.APIKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("gemini: create client: %w", err)
	}

	return &Gemini{client: client, model: cfg.Model, timeout: cfg.Timeout}, nil
}

// Name reports the provider and model.
func (g *Gemini) Name() string {
	return "Gemini/" + g.model
}

// Summarize compresses text into a short summary.
func (g *Gemini) Summarize(ctx context.Context, text string) (string, error) {
	if text == "" {
		return "", fmt.Errorf("%s: summarize: empty text", g.Name())
	}

	ctx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()

	cfg := &genai.GenerateContentConfig{
		SystemInstruction: genai.NewContentFromText(summarizePrompt, genai.RoleUser),
	}

	resp, err := g.client.Models.GenerateContent(ctx, g.model, genai.Text(text), cfg)
	if err != nil {
		return "", fmt.Errorf("%s: summarize: %w", g.Name(), err)
	}
	if len(resp.Candidates) == 0 {
		return "", fmt.Errorf("%s: summarize: no candidates", g.Name())
	}

	out := resp.Text()
	if out == "" {
		return "", fmt.Errorf("%s: summarize: empty response", g.Name())
	}
	return out, nil
}
