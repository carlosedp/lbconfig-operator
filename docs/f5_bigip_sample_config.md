# F5 BigIP Sample Config

## Master Nodes

```sh
# Create nodes
create ltm node openshift-master-1.myorg.com fqdn { name openshift-master-1.myorg.com }
create ltm node openshift-master-2.myorg.com fqdn { name openshift-master-2.myorg.com }
create ltm node openshift-master-3.myorg.com fqdn { name openshift-master-3.myorg.com }

# Create monitor
create ltm monitor https ocp-master-mon defaults-from https send "GET /healthz" destination "*.6443"

# Create VS for port 6443
create ltm pool master.myorg.com monitor ocp-master-mon members add { openshift-master-1.myorg.com:6443 openshift-master-2.myorg.com:6443 openshift-master-3.myorg.com.com:6443}
create ltm virtual OpenShift-Master pool master.myorg.com source-address-translation { type automap } destination 192.168.10.100:6443 profiles add { fastL4 }

# Create VS for port 22623
create ltm pool master.myorg.com monitor ocp-master-mon members add { openshift-master-1.myorg.com:22623 openshift-master-2.myorg.com:22623 openshift-master-3.myorg.com.com:22623}
create ltm virtual OpenShift-Master pool master.myorg.com source-address-translation { type automap } destination 192.168.10.100:22623 profiles add { fastL4 }
```

## Infra Nodes

```sh
# Create nodes
create ltm node openshift-infranode-1.myorg.com fqdn { name openshift-infranode-1.myorg.com }
create ltm node openshift-infranode-2.myorg.com fqdn { name openshift-infranode-2.myorg.com }
create ltm node openshift-infranode-3.myorg.com fqdn { name openshift-infranode-3.myorg.com }

# Create monitor
create ltm monitor http ocp-router defaults-from http send "GET /healthz" destination "*.1936"

# Create VS for port 80
create ltm pool infra.myorg.com-http monitor ocp-router members add { openshift-infranode-1.myorg.com:80 openshift-infranode-2.myorg.com:80 openshift-infranode-3.myorg.com:80 }
create ltm virtual infra.myorg.com-http  pool infra.myorg.com-http  persist replace-all-with { source_addr } source-address-translation { type automap } destination 192.168.10.101:80 profiles add { fastL4 }

# Create VS for port 443
create ltm pool infra.myorg.com-https monitor ocp-router members add { openshift-infranode-1.myorg.com:443 openshift-infranode-2.myorg.com:443 openshift-infranode-3.myorg.com:443 }
create ltm virtual infra.myorg.com-https pool infra.myorg.com-https persist replace-all-with { source_addr } source-address-translation { type automap } destination 192.168.10.101:443 profiles add { fastL4 }
```
