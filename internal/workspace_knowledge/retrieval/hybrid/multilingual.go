package hybrid

import (
	"math"
	"sort"
	"strings"
	"time"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval"
)

// MultilingualSearcherImpl versions the changed lexical semantics in replay.
const MultilingualSearcherImpl = "hybrid-v2-multilingual"

// NewMultilingual enables whole Unicode words, Latin accent folding and
// frequency-weighted title/tag + description matching. New remains v1 for replay.
func NewMultilingual(lister AssetLister) *Searcher {
	return &Searcher{Lister: lister, Clock: time.Now, multilingual: true}
}

type lexicalDocument struct {
	title, description string
	primary, body      map[string]struct{}
}

type rankAsset func(*assets.Asset) (float64, retrieval.ScoreBreakdown, string)

// Title/tag frequencies are local to eligible, tenant-scoped candidates. Prose
// mentions do not dilute the importance of a subject named in a heading. Body
// matches contribute less than headings, without a language-specific stoplist.
func multilingualRanker(query string, candidates []*assets.Asset, tags []string, now time.Time) rankAsset {
	documents, frequencies := indexLexicalDocuments(candidates, tags)
	terms := make([]string, 0)
	for term := range multilingualTerms(query) {
		terms = append(terms, term)
	}
	sort.Strings(terms)
	weights := make([]float64, 0, len(terms))
	total := 0.0
	for _, term := range terms {
		weight := 1 + math.Log(1+float64(len(documents))/float64(1+frequencies[term]))
		weights, total = append(weights, weight), total+weight
	}
	phrase := retrieval.NormalizeLexical(query)
	return func(asset *assets.Asset) (float64, retrieval.ScoreBreakdown, string) {
		text, overlap, hasPhrase := matchLexicalDocument(documents[asset], terms, weights, total, phrase)
		// Retain the existing boost/pin/recency formula and trace fields.
		_, breakdown, _ := scoreAsset(asset, nil, "", now)
		breakdown.TextMatch = 0.5 * text
		score := retrieval.Clamp01(breakdown.TextMatch + breakdown.Boost + breakdown.Pin + breakdown.Recency)
		reason := buildReason(overlap, hasPhrase, breakdown.Recency > 0, asset.Pinned, asset.Boost)
		return score, breakdown, strings.Replace(reason, "exact-phrase", "normalized-phrase", 1)
	}
}

func indexLexicalDocuments(candidates []*assets.Asset, tags []string) (map[*assets.Asset]lexicalDocument, map[string]int) {
	documents := make(map[*assets.Asset]lexicalDocument)
	frequencies := map[string]int{}
	for _, asset := range candidates {
		if asset.Type == assets.AssetBannedClaim || !filterByTags(asset, tags) {
			continue
		}
		primary := multilingualTerms(asset.Title)
		for _, tag := range asset.Tags {
			for term := range multilingualTerms(tag) {
				primary[term] = struct{}{}
			}
		}
		doc := lexicalDocument{retrieval.NormalizeLexical(asset.Title), retrieval.NormalizeLexical(asset.Description),
			primary, multilingualTerms(asset.Description)}
		documents[asset] = doc
		for term := range doc.primary {
			frequencies[term]++
		}
	}
	return documents, frequencies
}

func matchLexicalDocument(doc lexicalDocument, terms []string, weights []float64, total float64, phrase string) (float64, int, bool) {
	matched, overlap := 0.0, 0
	for index, term := range terms {
		weight := weights[index]
		if _, exists := doc.primary[term]; exists {
			matched += weight
			overlap++
		} else if _, exists := doc.body[term]; exists {
			matched += 0.35 * weight
			overlap++
		}
	}
	text := 0.0
	if total > 0 {
		text = matched / total
	}
	bonus := 0.0
	if len(weights) > 0 && containsWords(doc.title, phrase) {
		bonus = 1
	} else if len(weights) > 0 && containsWords(doc.description, phrase) {
		bonus = 0.5
	}
	return math.Max(text, bonus), overlap, bonus > 0
}

func (s *Searcher) ranker(query string, candidates []*assets.Asset, tags []string) rankAsset {
	now := s.now()
	if s.multilingual {
		return multilingualRanker(query, candidates, tags, now)
	}
	tokens := retrieval.Tokenize(query)
	phrase := strings.ToLower(strings.TrimSpace(query))
	return func(asset *assets.Asset) (float64, retrieval.ScoreBreakdown, string) {
		return scoreAsset(asset, tokens, phrase, now)
	}
}

func containsWords(text, phrase string) bool {
	return phrase != "" && strings.Contains(" "+text+" ", " "+phrase+" ")
}

// Adjacent word pairs preserve useful subjects such as "kho Mỹ". Title/tag
// frequency discounts repeated generic pairs such as "dịch vụ". Never create
// pairs across separate metadata fields or skip over one-character words.
func multilingualTerms(text string) map[string]struct{} {
	terms := retrieval.LexicalTokens(text)
	words := strings.Fields(retrieval.NormalizeLexical(text))
	for i := 1; i < len(words); i++ {
		_, left := terms[words[i-1]]
		_, right := terms[words[i]]
		if left && right {
			terms[words[i-1]+" "+words[i]] = struct{}{}
		}
	}
	return terms
}

// SearcherName lets fallback traces identify the actual lexical version.
func (s *Searcher) SearcherName() string {
	if s.multilingual {
		return MultilingualSearcherImpl
	}
	return SearcherImpl
}
