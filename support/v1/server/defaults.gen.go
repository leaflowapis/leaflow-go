// Code generated from the contract's servers[0]. DO NOT EDIT.
package supportv1server

// New 使用契约地址和原生 SecuritySource；不保留旧 ClientWithResponses 形状。
func New(security SecuritySource, options ...ClientOption) (*Client, error) {
	return NewClient("https://support.leaflow.cloud", security, options...)
}
