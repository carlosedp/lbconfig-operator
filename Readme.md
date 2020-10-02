# External Load Balancer Operator

**This is still a work-in-progress project.**

This operator manages external Load Balancer instances and creates VIPs and IP Pools with Monitors for the Master and Infra nodes based on it's roles and/or labels.

The IPs are updated automatically based on the Node IPs for each role. The objective is to have a modular architecture to allow plugging additional backends for different providers.

## Who is it for

The main users for this project is enterprise deployments or high-available clusters composed of multiple nodes having an external load-balancer providing the balancing and high-availability to access the cluster in both API and Application levels.

## High level architecture

```sh
+-----------------------------------------------------------------------------+
|          Nodes                                                              |
|                                                                             |
|   +-------------+                                                           |
|   |             |                                                           |
|   |   +-------------+                                                       |
|   |   |         |   |                                                       |
|   |   |   +--------------+                                                  |
|   +-------------+   |    |                                                  |
|       |   |         |    |                                                  |
|       +-------------+    |                                                  |
|           |              |                                                  |
|           +---+----------+                                                  |
|               ^                                                             |
|               |                                                             |
|               |                                                             |
|               |                                                             |
|   +---------------------------------------------------------------------+   |
|   |           |                                                         |   |       +-------------------+
|   |     +-----+------------------+       +------------------------+     |   |       |                   |
|   |     |                        |       |                        |     |   |       |                   |
|   |     |  ExternalLoadBalancer  +------>+  LoadBalancerBackend   +---------------->+   Load Balancer   |
|   |     |        Instance        |       |        Instance        |     |   |       |                   |
|   |     |                        |       |                        |     |   |       |                   |
|   |     +------------------------+       +-----------+------------+     |   |       +-------------------+
|   |                                                  |                  |   |
|   |                                                  |                  |   |
|   |                                                  |                  |   |
|   |                                                  v                  |   |
|   |                                           +------+------+           |   |
|   |                                           |             |           |   |
|   |                                           |   Secret    |           |   |
|   |                                           | Credentials |           |   |
|   |                                           |             |           |   |
|   |                                           +-------------+           |   |
|   |                                                                     |   |
|   |                              Operator                               |   |
|   +---------------------------------------------------------------------+   |
|                                                                             |
|                       Kubernetes / Openshift Cluster                        |
+-----------------------------------------------------------------------------+
```

## Planned Features

* Multiple backends (not in priority order)
  * [x] F5 BigIP
  * [x] Citrix ADC (Netscaler)
  * [ ] NGINX
  * [ ] HAProxy
  * [ ] NSX
* [ ] Dynamic port configuration from NodePort services

## Install

### Deploy the Operator to your cluster

> TBD

### Create ExternalLoadBalancer instances

First create a Load Balancer backend:

```yaml
apiVersion: lb.lbconfig.io/v1
kind: LoadBalancerBackend
metadata:
  name: backend-f5-sample
  namespace: lbconfig-operator-system
spec:
  provider:
    vendor: F5
    host: "192.168.1.35"
    port: 443
    creds: f5-creds
    partition: "Common"
    validatecerts: no
```

The provider `vendor` field can be:

* F5
* netscaler

And the secret holding the Load Balancer API user and password:

```sh
oc create secret generic f5-creds --from-literal=username=admin --from-literal=password=admin123 --namespace lbconfig-operator-system
```

Then create the instances for each Load Balancer you need (for example one for Master Nodes and another for the Infra Nodes):

The yaml field `type: "master"` or `type: "infra"` selects nodes with the role label `"node-role.kubernetes.io/master"` and `"node-role.kubernetes.io/infra"` respectively. If the field is ommited, the nodes will be selected only by the `nodelabels` labels.

Master Nodes:

```yaml
apiVersion: lb.lbconfig.io/v1
kind: ExternalLoadBalancer
metadata:
  name: externalloadbalancer-master-sample
  namespace: lbconfig-operator-system
spec:
  vip: "192.168.1.40"
  type: "master"
  backend: "backend-f5-sample"
  ports:
    - 6443
  monitor:
    path: "/healthz"
    port: 6443
    monitortype: "https"
```

Infra Nodes:

```yaml
apiVersion: lb.lbconfig.io/v1
kind: ExternalLoadBalancer
metadata:
  name: externalloadbalancer-infra-sample-shard
  namespace: lbconfig-operator-system
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

Infra Nodes with sharded routers are also supported. Create the YAML adding the `nodelabels` field with your node labels.

```yaml
spec:
  ...
  nodelabels:
    "node.kubernetes.io/region": "production"
```
