// Code generated from the contract's servers[0]. DO NOT EDIT.

package dnsv1

const defaultServer = "https://dns.leaflow.cloud"

// New returns a Client for the dns service. Pass WithBaseURL to override the address.
func New(opts ...ClientOption) (*Client, error) {
	return NewClient(defaultServer, opts...)
}

// NewWithResponses returns a ClientWithResponses for the dns service.
func NewWithResponses(opts ...ClientOption) (*ClientWithResponses, error) {
	return NewClientWithResponses(defaultServer, opts...)
}
