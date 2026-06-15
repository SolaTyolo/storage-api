package engine

import "testing"

func TestParseBucketRef(t *testing.T) {
	tests := []struct {
		defaultEng, ref, wantEng, wantBucket string
	}{
		{"rustfs", "uploads", "rustfs", "uploads"},
		{"rustfs", "minio:avatars", "minio", "avatars"},
		{"rustfs", "rustfs:uploads", "rustfs", "uploads"},
		{"rustfs", "", "rustfs", ""},
	}
	for _, tc := range tests {
		eng, bucket := ParseBucketRef(tc.defaultEng, tc.ref)
		if eng != tc.wantEng || bucket != tc.wantBucket {
			t.Fatalf("ParseBucketRef(%q, %q) = (%q, %q), want (%q, %q)",
				tc.defaultEng, tc.ref, eng, bucket, tc.wantEng, tc.wantBucket)
		}
	}
}

func TestFormatBucketID(t *testing.T) {
	if got := FormatBucketID("rustfs", "rustfs", "uploads"); got != "uploads" {
		t.Fatalf("default engine: got %q", got)
	}
	if got := FormatBucketID("rustfs", "minio", "avatars"); got != "minio:avatars" {
		t.Fatalf("non-default engine: got %q", got)
	}
}
