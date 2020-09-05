package backend

import "context"

// Provider interface method signatures
type Provider interface {
	Connect() error
	GetMonitor()
	GetPool()
	GetVIP()

	CreateMonitor(ctx context.Context, name string, url string, port int) (string, error)
	EditMonitor(ctx context.Context, name string, url string, port int) (string, error)
	DeleteMonitor(ctx context.Context, name string, url string, port int) (string, error)

	CreateMember(ctx context.Context, node string, IP string) (string, error)
	EditPoolMember(ctx context.Context, name string, member string, port int, status string) (string, error)
	DeletePoolMember(ctx context.Context, name string, member string, port int, status string) (string, error)

	CreatePool(ctx context.Context, name string, monitor string, members []string, port int) (string, error)
	EditPool(ctx context.Context, name string, monitor string, members []string, port int) (string, error)
	DeletePool(ctx context.Context, name string, monitor string, members []string, port int) (string, error)

	CreateVIP(ctx context.Context, name string, VIP string, pool string, port int) (string, error)
	EditVIP(ctx context.Context, name string, VIP string, pool string, port int) (string, error)
	DeleteVIP(ctx context.Context, name string, VIP string, pool string, port int) (string, error)
}

// ProviderOptions contains the connection parameters for the Provider
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
	// Check if monitor exists

	// Create Monitor

	// or Update Monitor ports and parameters
	return nil
}

//HandlePool manages the Pool validation, update and creation
func HandlePool(name string, nodeIPs map[string]string, port int) error {
	// Check if pool exists

	// if doesn't exist, create pool

	// Check pool members

	// Create pool members that do not exist

	// update pool adding new members and removing not used ones

	return nil
}

//HandleVIP manages the VIP validation, update and creation
func HandleVIP() error {
	// Check if VIP exists

	// if doesn't exist, create VIP

	// attach pool

	// update VIP ports and parameters

	return nil
}

func createMember(members map[string]string) error {
	for k, v := range members {
		nodeName := k
		IP := v
		_, _ = nodeName, IP
	}
	return nil
}
