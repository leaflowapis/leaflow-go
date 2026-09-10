// Code generated from the contract's servers[0]. DO NOT EDIT.

package fleetv1

const defaultServer = "https://fleet.leaflow.cloud"

// New returns a Client for the fleet service. Pass WithBaseURL to override the address.
func New(opts ...ClientOption) (*Client, error) {
	return NewClient(defaultServer, opts...)
}

// NewWithResponses returns a ClientWithResponses for the fleet service.
func NewWithResponses(opts ...ClientOption) (*ClientWithResponses, error) {
	return NewClientWithResponses(defaultServer, opts...)
}
