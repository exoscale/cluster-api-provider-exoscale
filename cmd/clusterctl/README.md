# Clusterctl

## Bootstrap cluster

```
go run cmd/clusterctl/main.go create cluster --vm-driver=hyperkit \
-m cmd/clusterctl/examples/exoscale/machine.yaml \
-c cmd/clusterctl/examples/exoscale/cluster.yaml \
-p provider-components.yaml \
--provider exoscale \
-v 9999
```