package backend_loader

import (
	// Load backend modules to register them in the manager
	_ "github.com/carlosedp/lbconfig-operator/controllers/backend/dummy"
	_ "github.com/carlosedp/lbconfig-operator/controllers/backend/f5"
	_ "github.com/carlosedp/lbconfig-operator/controllers/backend/haproxy"
	_ "github.com/carlosedp/lbconfig-operator/controllers/backend/netscaler"
)
