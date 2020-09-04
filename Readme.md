# External Load Balancer Operator

**This is still a work-in-progress project. It's still in non-functional state.**

This operator manages external Load Balancer instances and creates VIP and IP Pools for the Master and Infra nodes based on it's roles.

The IPs are updated automatically based on the Node IPs for each role.

Supported Load Balancer backends:

* F5 Big IP

