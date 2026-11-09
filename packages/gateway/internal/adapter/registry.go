package adapter

import (
	"fmt"
	"sort"

	"github.com/SolaTyolo/storage-api/internal/config"
)

// Registry maps bucket name → ObjectAdapter.
type Registry struct {
	adapters map[string]ObjectAdapter
}

// NewRegistry builds adapters for all configured buckets.
func NewRegistry(cfg *config.Config) (*Registry, error) {
	adapters := make(map[string]ObjectAdapter, len(cfg.Buckets))
	for _, b := range cfg.Buckets {
		ad, err := NewMinioAdapter(b)
		if err != nil {
			return nil, err
		}
		adapters[b.Name] = ad
	}
	return NewRegistryFrom(adapters), nil
}

// NewRegistryFrom wraps prebuilt adapters (useful for tests and custom backends).
func NewRegistryFrom(adapters map[string]ObjectAdapter) *Registry {
	return &Registry{adapters: adapters}
}

// Get returns the adapter for a bucket name.
func (r *Registry) Get(bucket string) (ObjectAdapter, error) {
	ad, ok := r.adapters[bucket]
	if !ok {
		return nil, fmt.Errorf("unknown bucket %q", bucket)
	}
	return ad, nil
}

// Buckets returns configured bucket names in sorted order.
func (r *Registry) Buckets() []string {
	names := make([]string, 0, len(r.adapters))
	for name := range r.adapters {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
