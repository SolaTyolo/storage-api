// Package objsdk is the Go client for obj-gateway.
//
// Business services authenticate with JWT, request short-lived presigned URLs
// from the gateway, and transfer object bytes directly to S3-compatible storage.
// The gateway never proxies PUT/GET bodies.
package objsdk
