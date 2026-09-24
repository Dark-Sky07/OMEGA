package service

// wgServerSettings is the smallest valid plain-WireGuard settings document
// used by the client CRUD tests. The production allocator treats the subnet
// fields as optional and falls back to 10.0.0.0/24, but spelling them out here
// keeps cross-inbound collision tests deterministic.
func wgServerSettings() string {
	return `{"subnetIp":"10.0.0.0","subnetCidr":24,"clients":[]}`
}
