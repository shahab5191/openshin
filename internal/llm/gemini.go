// Package llm holds the provider implementations openshin uses for the parts of
// the pipeline that need a model: summarising, embedding, and extracting.
// The agent itself is the brain; openshin only calls the model for these narrow,
// well-defined tasks.
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"google.golang.org/genai"
)

const (
	defaultGeminiModel       = "gemini-2.5-flash"
	defaultEmbeddingModel    = "text-embedding-004"
	defaultGeminiTimeout     = 60 * time.Second
	retrievalDocumentTask    = "RETRIEVAL_DOCUMENT"
	retrievalQueryTask       = "RETRIEVAL_QUERY"
)

// summarizePrompt asks the model to compress a stretch of conversation into a
// short, self-contained summary for the rolling short-term window.
const summarizePrompt = `You compress a stretch of chat messages into a short summary for a rolling memory window.

Rules:
- Keep only what a future turn would need to know: decisions, facts, and open threads.
- Write plain prose, as short as possible. Output only the summary.`

// extractSubjectPrompt asks for the one-sentence subject of the current
// conversation, used to detect when a segment ends.
const extractSubjectPrompt = `State in one short sentence what this conversation is about right now. Output only the sentence.`

// extractFactsPrompt asks the model to distil durable, self-contained facts from
// a finished segment of conversation. An empty list is a valid answer.
const extractFactsPrompt = `You extract durable, self-contained facts from a finished segment of conversation for a memory store.

Rules:
- Each fact must stand alone, without referring to "the user above" or the conversation itself.
- Do not include greetings, acknowledgments, small talk, or meta-commentary.
- Return a JSON object with a single "facts" array of strings.
- Return an empty array if the segment contains nothing worth remembering.`

// extractFactsSchema pins the facts output to {"facts": ["..."]}. The array may
// be empty.
var extractFactsSchema = &genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"facts": {Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeString}},
	},
	Required: []string{"facts"},
}

// extractLongTermPrompt asks the model to distil durable, role-tagged knowledge
// worth keeping beyond the decay window.
const extractLongTermPrompt = `You extract durable knowledge from a segment of conversation for a long-term memory store.

For each thing worth remembering, classify it as exactly one of:
- "fact": a stable truth about the user's world.
- "decision": a choice that was made, and its reasoning.
- "constraint": a boundary or rule that must not be violated.

Return a JSON object with an "items" array of {"role_tag", "content"}. Each content must be self-contained. Return an empty array if nothing is worth keeping long-term.`

var extractLongTermSchema = &genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"items": {
			Type: genai.TypeArray,
			Items: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"role_tag": {Type: genai.TypeString},
					"content":  {Type: genai.TypeString},
				},
				Required: []string{"role_tag", "content"},
			},
		},
	},
	Required: []string{"items"},
}

// TaggedItem is one role-tagged piece of long-term knowledge.
type TaggedItem struct {
	RoleTag string `json:"role_tag"`
	Content string `json:"content"`
}

// Skill is a reusable procedure extracted from AI activity. An empty Name means
// the conversation contained nothing reusable.
type Skill struct {
	Name    string `json:"name"`
	Trigger string `json:"trigger"`
	Steps   string `json:"steps"`
}

// extractSkillPrompt asks the model to generalise a subagent conversation into
// a reusable procedure.
const extractSkillPrompt = `You generalise an AI worker's conversation into a reusable procedure.

If the conversation shows how a problem was solved (a procedure another task could reuse), return a JSON object with:
- "name": a short, specific label for the procedure.
- "trigger": when to use it.
- "steps": the concrete steps taken.

If there is nothing reusable, return all three as empty strings.`

var extractSkillSchema = &genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"name":    {Type: genai.TypeString},
		"trigger": {Type: genai.TypeString},
		"steps":   {Type: genai.TypeString},
	},
	Required: []string{"name", "trigger", "steps"},
}

// GeminiConfig carries the settings for the Gemini provider. Zero values fall
// back to defaults.
type GeminiConfig struct {
	APIKey         string
	Model          string
	EmbeddingModel string
	Timeout        time.Duration
}

// GeminiConfigFromEnv reads the provider settings from the environment. Only the
// API key is required.
func GeminiConfigFromEnv() (GeminiConfig, error) {
	cfg := GeminiConfig{
		APIKey:         os.Getenv("GEMINI_API_KEY"),
		Model:          os.Getenv("GEMINI_MODEL"),
		EmbeddingModel: os.Getenv("EMBEDDING_MODEL"),
	}
	if cfg.APIKey == "" {
		return GeminiConfig{}, fmt.Errorf("gemini: GEMINI_API_KEY is not set")
	}
	return cfg, nil
}

// Gemini is the Gemini Developer API provider.
type Gemini struct {
	client         *genai.Client
	model          string
	embeddingModel string
	timeout        time.Duration
}

// NewGemini builds a Gemini provider.
func NewGemini(ctx context.Context, cfg GeminiConfig) (*Gemini, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("gemini: api key is required")
	}
	if cfg.Model == "" {
		cfg.Model = defaultGeminiModel
	}
	if cfg.EmbeddingModel == "" {
		cfg.EmbeddingModel = defaultEmbeddingModel
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

	return &Gemini{
		client:         client,
		model:          cfg.Model,
		embeddingModel: cfg.EmbeddingModel,
		timeout:        cfg.Timeout,
	}, nil
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
	return g.generateText(ctx, summarizePrompt, text)
}

// ExtractSubject returns a one-sentence subject for the conversation window.
func (g *Gemini) ExtractSubject(ctx context.Context, text string) (string, error) {
	if text == "" {
		return "", fmt.Errorf("%s: extract subject: empty text", g.Name())
	}
	s, err := g.generateText(ctx, extractSubjectPrompt, text)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(s), nil
}

// ExtractFacts distils durable facts from a finished segment. The returned slice
// may be empty.
func (g *Gemini) ExtractFacts(ctx context.Context, text string) ([]string, error) {
	if text == "" {
		return nil, fmt.Errorf("%s: extract facts: empty text", g.Name())
	}

	ctx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()

	cfg := &genai.GenerateContentConfig{
		SystemInstruction: genai.NewContentFromText(extractFactsPrompt, genai.RoleUser),
		ResponseMIMEType:  "application/json",
		ResponseSchema:    extractFactsSchema,
	}

	resp, err := g.client.Models.GenerateContent(ctx, g.model, genai.Text(text), cfg)
	if err != nil {
		return nil, fmt.Errorf("%s: extract facts: %w", g.Name(), err)
	}
	if len(resp.Candidates) == 0 {
		return nil, fmt.Errorf("%s: extract facts: no candidates", g.Name())
	}

	raw := strings.TrimSpace(resp.Text())
	if raw == "" {
		return nil, fmt.Errorf("%s: extract facts: empty response", g.Name())
	}

	var out struct {
		Facts []string `json:"facts"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("%s: extract facts: decode: %w", g.Name(), err)
	}
	return out.Facts, nil
}

// ExtractLongTerm distils durable, role-tagged knowledge from a segment. The
// returned slice may be empty.
func (g *Gemini) ExtractLongTerm(ctx context.Context, text string) ([]TaggedItem, error) {
	if text == "" {
		return nil, fmt.Errorf("%s: extract long-term: empty text", g.Name())
	}

	ctx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()

	cfg := &genai.GenerateContentConfig{
		SystemInstruction: genai.NewContentFromText(extractLongTermPrompt, genai.RoleUser),
		ResponseMIMEType:  "application/json",
		ResponseSchema:    extractLongTermSchema,
	}

	resp, err := g.client.Models.GenerateContent(ctx, g.model, genai.Text(text), cfg)
	if err != nil {
		return nil, fmt.Errorf("%s: extract long-term: %w", g.Name(), err)
	}
	if len(resp.Candidates) == 0 {
		return nil, fmt.Errorf("%s: extract long-term: no candidates", g.Name())
	}

	raw := strings.TrimSpace(resp.Text())
	if raw == "" {
		return nil, fmt.Errorf("%s: extract long-term: empty response", g.Name())
	}

	var out struct {
		Items []TaggedItem `json:"items"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("%s: extract long-term: decode: %w", g.Name(), err)
	}
	return out.Items, nil
}

// ExtractSkill generalises a subagent conversation into a reusable procedure.
func (g *Gemini) ExtractSkill(ctx context.Context, text string) (Skill, error) {
	if text == "" {
		return Skill{}, fmt.Errorf("%s: extract skill: empty text", g.Name())
	}

	ctx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()

	cfg := &genai.GenerateContentConfig{
		SystemInstruction: genai.NewContentFromText(extractSkillPrompt, genai.RoleUser),
		ResponseMIMEType:  "application/json",
		ResponseSchema:    extractSkillSchema,
	}

	resp, err := g.client.Models.GenerateContent(ctx, g.model, genai.Text(text), cfg)
	if err != nil {
		return Skill{}, fmt.Errorf("%s: extract skill: %w", g.Name(), err)
	}
	if len(resp.Candidates) == 0 {
		return Skill{}, fmt.Errorf("%s: extract skill: no candidates", g.Name())
	}

	raw := strings.TrimSpace(resp.Text())
	if raw == "" {
		return Skill{}, fmt.Errorf("%s: extract skill: empty response", g.Name())
	}

	var out Skill
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return Skill{}, fmt.Errorf("%s: extract skill: decode: %w", g.Name(), err)
	}
	return out, nil
}

// Embed computes a 768-dimension embedding for text.
func (g *Gemini) Embed(ctx context.Context, text string) ([]float32, error) {
	if text == "" {
		return nil, fmt.Errorf("%s: embed: empty text", g.Name())
	}

	ctx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()

	resp, err := g.client.Models.EmbedContent(ctx, g.embeddingModel,
		genai.Text(text), &genai.EmbedContentConfig{TaskType: retrievalDocumentTask})
	if err != nil {
		return nil, fmt.Errorf("%s: embed: %w", g.Name(), err)
	}
	if len(resp.Embeddings) == 0 || len(resp.Embeddings[0].Values) == 0 {
		return nil, fmt.Errorf("%s: embed: no embedding returned", g.Name())
	}
	return resp.Embeddings[0].Values, nil
}

// generateText runs a single plain-text generation with the given system prompt.
func (g *Gemini) generateText(ctx context.Context, systemPrompt, text string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()

	cfg := &genai.GenerateContentConfig{
		SystemInstruction: genai.NewContentFromText(systemPrompt, genai.RoleUser),
	}

	resp, err := g.client.Models.GenerateContent(ctx, g.model, genai.Text(text), cfg)
	if err != nil {
		return "", fmt.Errorf("%s: generate: %w", g.Name(), err)
	}
	if len(resp.Candidates) == 0 {
		return "", fmt.Errorf("%s: generate: no candidates", g.Name())
	}

	out := resp.Text()
	if out == "" {
		return "", fmt.Errorf("%s: generate: empty response", g.Name())
	}
	return out, nil
}
