package adapter

import (
	"fmt"

	"github.com/SolaTyolo/storage-api/internal/config"
)

// Registry maps bucket name → ObjectAdapter.
type Registry struct {
	adapters map[string]ObjectAdapter
}

// NewRegistry builds adapters for all configured buckets.
func NewRegistry(cfg *config.Config) (*Registry, error) {
	r := &Registry{adapters: make(map[string]ObjectAdapter, len(cfg.Buckets))}
	for _, b := range cfg.Buckets {
		ad, err := NewMinioAdapter(b)
		if err != nil {
			return nil, err
		}
		r.adapters[b.Name] = ad
	}
	return r, nil
}

// Get returns the adapter for a bucket name.
func (r *Registry) Get(bucket string) (ObjectAdapter, error) {
	ad, ok := r.adapters[bucket]
	if !ok {
		return nil, fmt.Errorf("unknown bucket %q", bucket)
	}
	return ad, nil
}
