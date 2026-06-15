package engine

import "strings"

// ParseBucketRef splits "engine:bucket" or plain "bucket" using defaultEngine when omitted.
func ParseBucketRef(defaultEngine, ref string) (engineName, bucketName string) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return defaultEngine, ""
	}
	if i := strings.Index(ref, ":"); i > 0 {
		return ref[:i], ref[i+1:]
	}
	return defaultEngine, ref
}

// FormatBucketID returns the Supabase-facing bucket id.
func FormatBucketID(defaultEngine, engineName, bucketName string) string {
	if engineName == defaultEngine || engineName == "" {
		return bucketName
	}
	return engineName + ":" + bucketName
}
