package engine

import (
	"context"
	"fmt"
	"strings"
)

// Registry routes bucket ids to physical engines.
type Registry struct {
	defaultEngine string
	engines       map[string]Engine
}

func NewRegistry(defaultEngine string, engines map[string]Engine) (*Registry, error) {
	if defaultEngine == "" {
		return nil, fmt.Errorf("default engine is required")
	}
	if len(engines) == 0 {
		return nil, fmt.Errorf("at least one engine is required")
	}
	if _, ok := engines[defaultEngine]; !ok {
		return nil, fmt.Errorf("default engine %q is not configured", defaultEngine)
	}
	return &Registry{defaultEngine: defaultEngine, engines: engines}, nil
}

func (r *Registry) DefaultEngine() string { return r.defaultEngine }

func (r *Registry) EngineNames() []string {
	names := make([]string, 0, len(r.engines))
	for name := range r.engines {
		names = append(names, name)
	}
	return names
}

func (r *Registry) Engine(name string) (Engine, error) {
	eng, ok := r.engines[name]
	if !ok {
		return nil, fmt.Errorf("engine %q not found", name)
	}
	return eng, nil
}

func (r *Registry) Resolve(ctx context.Context, bucketRef string) (ResolvedBucket, Engine, error) {
	engineName, bucketName := ParseBucketRef(r.defaultEngine, bucketRef)
	if bucketName == "" {
		return ResolvedBucket{}, nil, fmt.Errorf("bucket name is required")
	}
	eng, err := r.Engine(engineName)
	if err != nil {
		return ResolvedBucket{}, nil, err
	}
	meta, _, err := eng.GetBucketMeta(ctx, bucketName)
	if err != nil {
		return ResolvedBucket{}, nil, err
	}
	return ResolvedBucket{
		Engine:           engineName,
		Bucket:           bucketName,
		DisplayID:        FormatBucketID(r.defaultEngine, engineName, bucketName),
		Public:           meta.Public,
		FileSizeLimit:    meta.FileSizeLimit,
		AllowedMimeTypes: meta.AllowedMimeTypes,
	}, eng, nil
}

func (r *Registry) ListAllBuckets(ctx context.Context) ([]ResolvedBucket, error) {
	var out []ResolvedBucket
	for name, eng := range r.engines {
		names, err := eng.ListBuckets(ctx)
		if err != nil {
			return nil, fmt.Errorf("engine %s: %w", name, err)
		}
		for _, b := range names {
			if strings.HasPrefix(b, ".__") {
				continue
			}
			meta, _, _ := eng.GetBucketMeta(ctx, b)
			out = append(out, ResolvedBucket{
				Engine:           name,
				Bucket:           b,
				DisplayID:        FormatBucketID(r.defaultEngine, name, b),
				Public:           meta.Public,
				FileSizeLimit:    meta.FileSizeLimit,
				AllowedMimeTypes: meta.AllowedMimeTypes,
			})
		}
	}
	return out, nil
}

func (r *Registry) Ping(ctx context.Context) error {
	for name, eng := range r.engines {
		if _, err := eng.ListBuckets(ctx); err != nil {
			return fmt.Errorf("engine %s: %w", name, err)
		}
	}
	return nil
}
