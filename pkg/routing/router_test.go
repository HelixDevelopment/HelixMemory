package routing

import (
	"testing"

	"digital.vasic.helixmemory/pkg/types"

	"github.com/stretchr/testify/assert"
)

func TestNewRouter(t *testing.T) {
	r := NewRouter()
	assert.NotNil(t, r)
	assert.Len(t, r.writePriority, 6)
	assert.Len(t, r.readSources, 4)
}

func TestRouter_ClassifyMemoryType(t *testing.T) {
	r := NewRouter()

	tests := []struct {
		name     string
		content  string
		expected types.MemoryType
	}{
		{
			name:     "temporal - yesterday",
			content:  "yesterday the user changed their preference",
			expected: types.MemoryTypeTemporal,
		},
		{
			name:     "temporal - ago",
			content:  "three days ago the API was updated",
			expected: types.MemoryTypeTemporal,
		},
		{
			name:     "procedural - how to",
			content:  "how to deploy the application step by step",
			expected: types.MemoryTypeProcedural,
		},
		{
			name:     "procedural - workflow",
			content:  "the build workflow process is automated",
			expected: types.MemoryTypeProcedural,
		},
		{
			name:     "graph - relates to",
			content:  "the auth service relates to the user database",
			expected: types.MemoryTypeGraph,
		},
		{
			name:     "graph - depends on",
			content:  "this module depends on the cache layer",
			expected: types.MemoryTypeGraph,
		},
		{
			name:     "core - persona",
			content:  "i am a software engineer who prefers Go",
			expected: types.MemoryTypeCore,
		},
		{
			name:     "episodic - conversation",
			content:  "in the conversation we discussed API design",
			expected: types.MemoryTypeEpisodic,
		},
		{
			name:     "default - fact",
			content:  "the temperature in Tokyo today reached 32 degrees",
			expected: types.MemoryTypeFact,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := r.ClassifyMemoryType(tt.content)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRouter_RouteWrite(t *testing.T) {
	r := NewRouter()

	tests := []struct {
		name     string
		entry    *types.MemoryEntry
		expected types.MemorySource
	}{
		{
			name:     "fact to mem0",
			entry:    &types.MemoryEntry{Type: types.MemoryTypeFact},
			expected: types.SourceMem0,
		},
		{
			name:     "graph to cognee",
			entry:    &types.MemoryEntry{Type: types.MemoryTypeGraph},
			expected: types.SourceCognee,
		},
		{
			name:     "core to letta",
			entry:    &types.MemoryEntry{Type: types.MemoryTypeCore},
			expected: types.SourceLetta,
		},
		{
			name:     "temporal to graphiti",
			entry:    &types.MemoryEntry{Type: types.MemoryTypeTemporal},
			expected: types.SourceGraphiti,
		},
		{
			name:     "episodic to letta",
			entry:    &types.MemoryEntry{Type: types.MemoryTypeEpisodic},
			expected: types.SourceLetta,
		},
		{
			name:     "procedural to cognee",
			entry:    &types.MemoryEntry{Type: types.MemoryTypeProcedural},
			expected: types.SourceCognee,
		},
		{
			name:     "auto-classify fact",
			entry:    &types.MemoryEntry{Content: "the temperature in Tokyo reached 32 degrees"},
			expected: types.SourceMem0,
		},
		{
			name:     "auto-classify temporal",
			entry:    &types.MemoryEntry{Content: "yesterday the config changed"},
			expected: types.SourceGraphiti,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := r.RouteWrite(tt.entry)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRouter_RouteRead(t *testing.T) {
	r := NewRouter()

	// No sources specified — returns all
	req := &types.SearchRequest{Query: "test"}
	sources := r.RouteRead(req)
	assert.Len(t, sources, 4)

	// Specific sources
	req = &types.SearchRequest{
		Query:   "test",
		Sources: []types.MemorySource{types.SourceMem0},
	}
	sources = r.RouteRead(req)
	assert.Len(t, sources, 1)
	assert.Equal(t, types.SourceMem0, sources[0])

	// Specific types — still returns all sources
	req = &types.SearchRequest{
		Query: "test",
		Types: []types.MemoryType{types.MemoryTypeFact},
	}
	sources = r.RouteRead(req)
	assert.GreaterOrEqual(t, len(sources), 4)
}

func TestRouter_SetWritePriority(t *testing.T) {
	r := NewRouter()

	// Override fact routing to Cognee
	r.SetWritePriority(types.MemoryTypeFact, types.SourceCognee)

	entry := &types.MemoryEntry{Type: types.MemoryTypeFact}
	assert.Equal(t, types.SourceCognee, r.RouteWrite(entry))
}

func TestRouter_SetReadSources(t *testing.T) {
	r := NewRouter()

	r.SetReadSources([]types.MemorySource{types.SourceMem0, types.SourceLetta})

	req := &types.SearchRequest{Query: "test"}
	sources := r.RouteRead(req)
	assert.Len(t, sources, 2)
}
