// Code generated from the contract's servers[0]. DO NOT EDIT.

package billingaccountv1

const defaultServer = "https://billing.leaflow.cloud"

// New returns a Client for the billing account service. Pass WithBaseURL to override the address.
func New(opts ...ClientOption) (*Client, error) {
	return NewClient(defaultServer, opts...)
}

// NewWithResponses returns a ClientWithResponses for the billing account service.
func NewWithResponses(opts ...ClientOption) (*ClientWithResponses, error) {
	return NewClientWithResponses(defaultServer, opts...)
}
