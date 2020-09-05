package backend

type BackendProvider interface {
	Connect() error
	GetMonitor()
	GetPool()
	GetVIP()
	CreateMonitor() (string, error)
	CreatePool() (string, error)
	CreateVIP() (string, error)
}

type Provider struct {
	providerVendor string
}

type ProviderOptions struct {
	host          string
	hostport      int
	username      string
	password      string
	partition     string
	validatecerts bool
}

//HandleMonitors manages the Monitor validation, update and creation
func HandleMonitors() error {
	return nil
}

//HandlePool manages the Pool validation, update and creation
func HandlePool(nodeIPs map[string]string) error {
	return nil
}

//HandleVIP manages the VIP validation, update and creation
func HandleVIP() error {
	return nil
}
