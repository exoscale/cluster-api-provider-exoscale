# Custom Resource Definitions

The ClusterAPI

## exoscaleclusterproviderspec

Stores the cluster configuration.

- `zone` - the zone name for the machine
- `sshKey` - the key name to setup within the machine

## exoscaleclusterproviderstatus

Stores the cluster dynamic configuration.

- `antiAffinityGroups`
- `securityGroups`
- `loadBalancer`
- etc.

## exoscalemachineproviderspec

- `existingSecurityGroups`
- `tags`
- `image`
- `size`
- `rootDiskSize`
- `IPv6`

## exoscalemachineproviderstatus

- `instanceID`
- `state`
- etc.
