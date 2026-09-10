// Code generated from the contract's servers[0]. DO NOT EDIT.

package computev1

const defaultServer = "https://compute.leaflow.cloud"

// New returns a Client for the compute service. Pass WithBaseURL to override the address.
func New(opts ...ClientOption) (*Client, error) {
	return NewClient(defaultServer, opts...)
}

// NewWithResponses returns a ClientWithResponses for the compute service.
func NewWithResponses(opts ...ClientOption) (*ClientWithResponses, error) {
	return NewClientWithResponses(defaultServer, opts...)
}
