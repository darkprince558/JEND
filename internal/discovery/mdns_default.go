//go:build darwin || windows

package discovery

func init() {
	DefaultMDNSProvider = &ZeroconfProvider{}
}
