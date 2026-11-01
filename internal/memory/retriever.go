package memory

import (
	"context"
	"math"
	"sort"
	"time"

	"github.com/shahab5191/openshin/internal/store"
)

// retrieverStore is what search needs: the active mid-term facts, the long-term
// items, and the skills, all carrying embeddings.
type retrieverStore interface {
	ListActiveFacts(ctx context.Context, sessionID, now string) ([]store.Fact, error)
	ListLongTerm(ctx context.Context, sessionID string) ([]store.LongTermItem, error)
	ListSkills(ctx context.Context, sessionID string) ([]store.Skill, error)
}

// SearchResult is one retrieved memory item, ranked by similarity.
type SearchResult struct {
	Content    string  `json:"content"`
	RoleTag    string  `json:"role_tag,omitempty"`
	StartSeq   int64   `json:"start_seq,omitempty"`
	EndSeq     int64   `json:"end_seq,omitempty"`
	Similarity float64 `json:"similarity"`
}

// Retriever ranks mid-term and long-term memories by cosine similarity to a
// query. It is brute-force and in-process — exact, and fine at local scale.
type Retriever struct {
	store retrieverStore
	embed embedder
}

func NewRetriever(store retrieverStore, embed embedder) *Retriever {
	return &Retriever{store: store, embed: embed}
}

// Search returns the top-K memories most similar to query, best first. Mid-term
// facts have no role tag; long-term items carry fact/decision/constraint.
func (r *Retriever) Search(ctx context.Context, sessionID, query string, topK int) ([]SearchResult, error) {
	if topK <= 0 {
		topK = 5
	}

	qEmb, err := r.embed.Embed(ctx, query)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	facts, err := r.store.ListActiveFacts(ctx, sessionID, now)
	if err != nil {
		return nil, err
	}
	items, err := r.store.ListLongTerm(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	skills, err := r.store.ListSkills(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	type candidate struct {
		content, roleTag string
		startSeq, endSeq int64
		sim              float64
	}

	cands := make([]candidate, 0, len(facts)+len(items)+len(skills))
	for _, f := range facts {
		cands = append(cands, candidate{
			content:  f.Fact,
			startSeq: f.StartSeq,
			endSeq:   f.EndSeq,
			sim:      cosine(qEmb, store.DecodeEmbedding(f.Embedding)),
		})
	}
	for _, it := range items {
		cands = append(cands, candidate{
			content:  it.Content,
			roleTag:  it.RoleTag,
			startSeq: it.StartSeq,
			endSeq:   it.EndSeq,
			sim:      cosine(qEmb, store.DecodeEmbedding(it.Embedding)),
		})
	}
	for _, sk := range skills {
		cands = append(cands, candidate{
			content: sk.Name + ": " + sk.Steps,
			roleTag: "skill",
			sim:     cosine(qEmb, store.DecodeEmbedding(sk.Embedding)),
		})
	}

	sort.Slice(cands, func(i, j int) bool { return cands[i].sim > cands[j].sim })
	if topK > len(cands) {
		topK = len(cands)
	}

	results := make([]SearchResult, 0, topK)
	for i := 0; i < topK; i++ {
		results = append(results, SearchResult{
			Content:    cands[i].content,
			RoleTag:    cands[i].roleTag,
			StartSeq:   cands[i].startSeq,
			EndSeq:     cands[i].endSeq,
			Similarity: cands[i].sim,
		})
	}
	return results, nil
}

// cosine is the cosine similarity of two embeddings of equal length.
func cosine(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
