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
