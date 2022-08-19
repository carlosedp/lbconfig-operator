# Adding new Backend Providers

The architecture of the operator allows an almost-seamless addition of new load-balancer backends without the need to change the logic of the operator.

Each backend is composed of a CRUD (Create-Read-Update-Delete) matrix that implements the essential methods to each element needed, a Monitor, a Server Pool, Server Pool Members and a VIP (or Virtual Server).

I advise creating a new issue tagged as [Feature] to discuss with the maintainer things like the package name, Vendor name and the needs for new fields in the configuration.

The easier method is to copy one of the existing providers to a new folder inside `controllers/backend` directory with the backend name and then import the modules or use the vendor API to do each method task.

To implement a new backend, the following steps are required:

1. Create a package directory at `controllers/backend` with provider name;
2. Create the provider code with CRUD matrix of functions implementing the `Provider` interface based on existing provider;
3. Create the test file using Ginkgo based on existing provider tests;
4. Add the new package to be loaded by the [`controllers/backend/backend_loader/backend_loader.go`](controllers/backend/backend_loader/backend_loader.go) as an `_` import. This registers the provider with the backend controller;
5. Add the new provider name (the name used in the `RegisterProvider`) to the Enum `Provider` -> `Vendor` at [`api/v1/externalloadbalancer_types.go`](api/v1/externalloadbalancer_types.go) so it will be allowed in the YAML CustomResource.
6. Each provider implements some load-balancing methods. The CustomResource YAML has some strict ones in an Enumeration. Your provider should map them to the correct names used by the new backend API. Check the F5 controller `LBMethodMap` variable.
7. If you think the new backend provides some additional function that could be user-configurable and requires a new field in the CustomResource YAML, discuss in the issue with the maintainer.

The new backend **should never touch other element than the ones it creates**. Also it's important to never delete the server (pool member) from the load balancer since this server could also be used on other server pools. If you prefer to do it, make sure you check that the server is not used anywhere with the vendor API.
