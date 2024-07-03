package backend_loader

import (
	// Load backend modules to register them in the manager
	_ "github.com/carlosedp/lbconfig-operator/internal/controller/backend/dummy"
	_ "github.com/carlosedp/lbconfig-operator/internal/controller/backend/f5"
	_ "github.com/carlosedp/lbconfig-operator/internal/controller/backend/haproxy"
	_ "github.com/carlosedp/lbconfig-operator/internal/controller/backend/netscaler"
)
