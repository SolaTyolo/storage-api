// Package objsdk is the Go client for obj-gateway.
//
// Business services authenticate with JWT, request short-lived presigned URLs
// from the gateway, and transfer object bytes directly to S3-compatible storage.
// The gateway never proxies PUT/GET bodies.
//
// For browser uploads, mint a scoped JWT and use obj-gateway-sdk, or call
// PresignPut and return the upload URL to the client.
package objsdk
