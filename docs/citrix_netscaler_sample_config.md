# Citrix Netscaler ADC Sample Config

## Master Nodes

```sh
# Create Monitor
add lb monitor MON_ocp-master HTTP-ECV -send "GET /healthz" -recv ok -LRTM DISABLED -secure YES

# Create ServiceGroup for port 443
add serviceGroup ose-console_443_sslbridge SSL_BRIDGE -maxClient 0 -maxReq 0 -cip DISABLED -usip NO -useproxyport YES -cltTimeout 180 -svrTimeout 360 -CKA YES -TCPB YES -CMP NO
add lb vserver ose-console_443_sslbridge SSL_BRIDGE 192.168.10.101 443 -persistenceType SSLSESSION -timeout 60 -cltTimeout 180

bind lb vserver ose-console_443_sslbridge ose-console_443_sslbridge

# Bind nodes
bind serviceGroup ose-console_443_sslbridge openshift-master-1.c1-ocp.myorg.com 443 -monitorName ocp-master
bind serviceGroup ose-console_443_sslbridge openshift-master-2.c1-ocp.myorg.com 443 -monitorName ocp-master
bind serviceGroup ose-console_443_sslbridge openshift-master-3.c1-ocp.myorg.com 443 -monitorName ocp-master

# Create ServiceGroup for port 22623
add serviceGroup ose-console_22623_sslbridge SSL_BRIDGE -maxClient 0 -maxReq 0 -cip DISABLED -usip NO -useproxyport YES -cltTimeout 180 -svrTimeout 360 -CKA YES -TCPB YES -CMP NO
add lb vserver ose-console_22623_sslbridge SSL_BRIDGE 192.168.10.101 22623 -persistenceType SSLSESSION -timeout 60 -cltTimeout 180

bind lb vserver ose-console_22623_sslbridge ose-console_443_sslbridge

# Bind nodes
bind serviceGroup ose-console_22623_sslbridge openshift-master-1.c1-ocp.myorg.com 22623 -monitorName ocp-master
bind serviceGroup ose-console_22623_sslbridge openshift-master-2.c1-ocp.myorg.com 22623 -monitorName ocp-master
bind serviceGroup ose-console_22623_sslbridge openshift-master-3.c1-ocp.myorg.com 22623 -monitorName ocp-master
```

## Infra Nodes

```sh
# Port 443
add lb monitor MON_ocp-router HTTP -respCode 200 -httpRequest "GET /healthz" -LRTM DISABLED -destPort 1936 -netProfile Monitor_SNIP
add serviceGroup ose-wildcard_443_sslbridge SSL_BRIDGE -maxClient 0 -maxReq 0 -cip DISABLED -usip NO -useproxyport YES -cltTimeout 180 -svrTimeout 360 -CKA YES -TCPB YES -CMP NO
add lb vserver ose-wildcard_443_sslbridge SSL_BRIDGE 192.168.10.102 443 -persistenceType SSLSESSION -timeout 60 -cltTimeout 180
bind lb vserver ose-wildcard_443_sslbridge ose-wildcard_443_sslbridge
bind serviceGroup ose-wildcard_443_sslbridge openshift-infranode-1.c1-ocp.myorg.com 443 -monitorName ocp-router
bind serviceGroup ose-wildcard_443_sslbridge openshift-infranode-2.c1-ocp.myorg.com 443 -monitorName ocp-router
bind serviceGroup ose-wildcard_443_sslbridge openshift-infranode-3.c1-ocp.myorg.com 443 -monitorName ocp-router

# Port 80
add serviceGroup ose-wildcard_80 http -maxClient 0 -maxReq 0 -cip DISABLED -usip NO -useproxyport YES -cltTimeout 180 -svrTimeout 360 -CKA YES -TCPB YES -CMP NO
add lb vserver ose-wildcard_80 192.168.10.102 80 -persistenceType SSLSESSION -timeout 60 -cltTimeout 180
bind lb vserver ose-wildcard_80 ose-wildcard_80
bind serviceGroup ose-wildcard_80 openshift-infranode-1.c1-ocp.myorg.com 80 -monitorName ocp-router
bind serviceGroup ose-wildcard_80 openshift-infranode-2.c1-ocp.myorg.com 80 -monitorName ocp-router
bind serviceGroup ose-wildcard_80 openshift-infranode-3.c1-ocp.myorg.com 80 -monitorName ocp-router
```
