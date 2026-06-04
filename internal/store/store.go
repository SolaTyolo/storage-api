package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/postship/storage/internal/model"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func (s *Store) CreateBucket(ctx context.Context, b model.Bucket) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO storage.buckets (id, name, owner, public, file_size_limit, allowed_mime_types)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, b.ID, b.Name, b.Owner, b.Public, b.FileSizeLimit, b.AllowedMimeTypes)
	return err
}

func (s *Store) GetBucket(ctx context.Context, id string) (model.Bucket, error) {
	var b model.Bucket
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, owner, public, file_size_limit, allowed_mime_types, created_at, updated_at
		FROM storage.buckets WHERE id = $1
	`, id).Scan(&b.ID, &b.Name, &b.Owner, &b.Public, &b.FileSizeLimit, &b.AllowedMimeTypes, &b.CreatedAt, &b.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return b, ErrNotFound
	}
	return b, err
}

func (s *Store) ListBuckets(ctx context.Context) ([]model.Bucket, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, owner, public, file_size_limit, allowed_mime_types, created_at, updated_at
		FROM storage.buckets ORDER BY created_at
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Bucket
	for rows.Next() {
		var b model.Bucket
		if err := rows.Scan(&b.ID, &b.Name, &b.Owner, &b.Public, &b.FileSizeLimit, &b.AllowedMimeTypes, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) UpsertObject(ctx context.Context, obj model.StorageObject) error {
	meta, err := json.Marshal(obj.Metadata.ToMap())
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO storage.objects (id, bucket_id, name, owner, metadata)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (bucket_id, name) DO UPDATE SET
			metadata = EXCLUDED.metadata,
			updated_at = NOW()
	`, obj.ID, obj.BucketID, obj.Name, obj.Owner, meta)
	return err
}

func (s *Store) GetObject(ctx context.Context, bucketID, name string) (model.StorageObject, error) {
	return s.getObject(ctx, `bucket_id = $1 AND name = $2`, bucketID, name)
}

func (s *Store) GetObjectByID(ctx context.Context, id uuid.UUID) (model.StorageObject, error) {
	return s.getObject(ctx, `id = $1`, id)
}

func (s *Store) getObject(ctx context.Context, where string, args ...any) (model.StorageObject, error) {
	var obj model.StorageObject
	var raw []byte
	q := fmt.Sprintf(`
		SELECT id, bucket_id, name, owner, created_at, updated_at, last_accessed_at, metadata
		FROM storage.objects WHERE %s
	`, where)
	err := s.pool.QueryRow(ctx, q, args...).Scan(
		&obj.ID, &obj.BucketID, &obj.Name, &obj.Owner,
		&obj.CreatedAt, &obj.UpdatedAt, &obj.LastAccessedAt, &raw,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return obj, ErrNotFound
	}
	if err != nil {
		return obj, err
	}
	var meta map[string]any
	_ = json.Unmarshal(raw, &meta)
	obj.Metadata = mapToMetadata(meta)
	return obj, nil
}

func (s *Store) ListObjects(ctx context.Context, bucketID, prefix string, limit int) ([]model.StorageObject, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, bucket_id, name, owner, created_at, updated_at, last_accessed_at, metadata
		FROM storage.objects
		WHERE bucket_id = $1 AND name LIKE $2
		ORDER BY name
		LIMIT $3
	`, bucketID, prefix+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.StorageObject
	for rows.Next() {
		var obj model.StorageObject
		var raw []byte
		if err := rows.Scan(&obj.ID, &obj.BucketID, &obj.Name, &obj.Owner, &obj.CreatedAt, &obj.UpdatedAt, &obj.LastAccessedAt, &raw); err != nil {
			return nil, err
		}
		var meta map[string]any
		_ = json.Unmarshal(raw, &meta)
		obj.Metadata = mapToMetadata(meta)
		out = append(out, obj)
	}
	return out, rows.Err()
}

func (s *Store) UpdateObjectMetadata(ctx context.Context, id uuid.UUID, meta model.ObjectMetadata) error {
	b, err := json.Marshal(meta.ToMap())
	if err != nil {
		return err
	}
	ct, err := s.pool.Exec(ctx, `
		UPDATE storage.objects SET metadata = $2, updated_at = NOW() WHERE id = $1
	`, id, b)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteObject(ctx context.Context, bucketID, name string) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM storage.objects WHERE bucket_id = $1 AND name = $2`, bucketID, name)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func mapToMetadata(m map[string]any) model.ObjectMetadata {
	b, _ := json.Marshal(m)
	var meta model.ObjectMetadata
	_ = json.Unmarshal(b, &meta)
	if v, ok := m["mimetype"].(string); ok {
		meta.MimeType = v
	}
	return meta
}
