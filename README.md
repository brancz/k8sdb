k8sdb
=====

An experimental approach on managing databases within a kubernetes cluster.

> Warning: this is not production suitable at all

It does ...

- Create X amount of instances
- Configure replication for those instances in a full mesh

It (currently) does not ...

- React to changes to a cluster
- Change a cluster
- Use persistent storage

It requires access to the kubernetes cluster to work in. Either outside of
cluster or within the cluster is supported. Outside the cluster works the same
way as configuring `kubectl`.

