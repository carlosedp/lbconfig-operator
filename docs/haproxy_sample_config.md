# HAProxy Sample Config


```
# Global settings
#---------------------------------------------------------------------
global
    maxconn     20000
    log         /dev/log local0 info
    chroot      /var/lib/haproxy
    pidfile     /var/run/haproxy.pid
    user        haproxy
    group       haproxy
    daemon

    # turn on stats unix socket
    stats socket /var/lib/haproxy/stats

#---------------------------------------------------------------------
# common defaults that all the 'listen' and 'backend' sections will
# use if not designated in their block
#---------------------------------------------------------------------
defaults
    mode                    http
    log                     global
    option                  httplog
    option                  dontlognull
#    option http-server-close
    option forwardfor       except 127.0.0.0/8
    option                  redispatch
    retries                 3
    timeout http-request    10s
    timeout queue           1m
    timeout connect         10s
    timeout client          300s
    timeout server          300s
    timeout http-keep-alive 10s
    timeout check           10s
    maxconn                 20000

listen stats
    bind :9000
    mode http
    stats enable
    stats uri /

# OpenShift 4.x
# Port 6443
frontend  openshift-api-server
    bind *:6443
    default_backend openshift-api-server
    mode tcp
    option tcplog

backend openshift-api-server
    balance source
    mode tcp
    server      bootstrap 10.36.72.11:6443 check
    server      master1   10.36.72.12:6443 check
    server      master2   10.36.72.13:6443 check
    server      master3   10.36.72.14:6443 check

# Port 22623
frontend  machine-config-server
    bind *:22623
    default_backend machine-config-server
    mode tcp
    option tcplog

backend machine-config-server
    balance source
    mode tcp
    server      bootstrap 10.36.72.11:22623 check
    server      master1   10.36.72.12:22623 check
    server      master2   10.36.72.13:22623 check
    server      master3   10.36.72.14:22623 check

# Ingress Http
frontend ingress-http
    bind *:80
    default_backend ingress-http
    mode tcp
    option tcplog

backend ingress-http
    balance source
    mode tcp
    server      worker1 10.36.72.xx:80 check
    server      worker2 10.36.72.xx:80 check

# Ingress HttpS
frontend ingress-https
    bind *:443
    default_backend ingress-https
    mode tcp
    option tcplog

backend ingress-https
    balance source
    mode tcp
    server      worker1 10.36.72.xx:443 check
    server      worker2 10.36.72.xx:443 check
```