package utilmock

// MockIPResolver is the mock ip resolver
type MockIPResolver struct {
	IP string
}

// GetPublicIP is the mock ip resolver's `GetPublicIP` func
func (m *MockIPResolver) GetPublicIP() (string, error) {
	return m.IP, nil
}
