package ipc

// Empty is an empty RPC request or response used by operations without
// additional payload.
type Empty struct{}

// BaseRequest carries a resource name in an RPC request.
type BaseRequest struct {
	// Name identifies the requested resource.
	Name string
}

// DeleteRequest is the common name-only request used by delete operations.
type DeleteRequest = BaseRequest
