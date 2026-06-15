package engine

import (
	"fmt"

	appconfig "github.com/SolaTyolo/storage-api/internal/config"
)

func LoadRegistry(storage appconfig.StorageYAML) (*Registry, error) {
	engines := make(map[string]Engine)
	for name, spec := range storage.Engines {
		switch spec.Type {
		case "s3", "":
			eng, err := NewS3Engine(name, S3Config{
				Endpoint:        spec.Endpoint,
				PublicEndpoint:  spec.PublicEndpoint,
				Region:          spec.Region,
				AccessKeyID:     spec.AccessKeyID,
				SecretAccessKey: spec.SecretAccessKey,
				PathStyle:       spec.PathStyle,
			})
			if err != nil {
				return nil, fmt.Errorf("engine %s: %w", name, err)
			}
			engines[name] = eng
		default:
			return nil, fmt.Errorf("engine %s: unsupported type %q", name, spec.Type)
		}
	}
	return NewRegistry(storage.DefaultEngine, engines)
}
