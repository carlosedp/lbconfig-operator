# External Load Balancer Operator

**This is still a work-in-progress project in non-functional state.**

This operator manages external Load Balancer instances and creates VIPs and IP Pools with monitoring for the Master and Infra nodes based on it's roles.

The IPs are updated automatically based on the Node IPs for each role. The objective is to have a modular architecture to allow plugging additional backends for different providers.

Supported Load Balancer backends:

* F5 Big IP

## Install

Deploy the Operator to your cluster

**TBD**

## Create ExternalLoadBalancer instances

First create a backend:

```yaml
apiVersion: lb.lbconfig.io/v1
kind: LoadBalancerBackend
metadata:
  name: backend-f5-sample
  namespace: lbconfig
spec:
  provider:
    vendor: F5
    host: "10.0.0.1"
    hostport: 443
    creds: "f5-creds"
    partition: "Common"
    validatecerts: no
```

And the secret holding the Load Balancer API user and password:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: f5-creds
  namespace: lbconfig
data:
  username: "admin"
  password: "admin123"
```

Then create the instances for each Load Balancer you need (for example one for Master Nodes and another for the Infra Nodes):

Master Nodes:

```yaml
apiVersion: lb.lbconfig.io/v1
kind: ExternalLoadBalancer
metadata:
  name: externalloadbalancer-master-sample
  namespace: lbconfig
spec:
  vip: "10.0.0.5"
  type: "master"
  backend: "backend-f5-sample"
  ports:
    - 6443
  monitor:
    path: "/healthz"
    port: 6443
```

Infra Nodes:

```yaml
apiVersion: lb.lbconfig.io/v1
kind: ExternalLoadBalancer
metadata:
  name: externalloadbalancer-infra-sample-shard
  namespace: lbconfig
spec:
  vip: "10.0.0.6"
  type: "infra"
  backend: "backend-f5-sample"
  ports:
    - 80
    - 443
  monitor:
    path: "/healthz"
    port: 1936
```

Infra Nodes with sharded routers are also supported. Create the YAML adding the `shardlabels` field with your node labels.

```yaml
spec:
  ...
  shardlabels:
    "node.kubernetes.io/region": "production"
```