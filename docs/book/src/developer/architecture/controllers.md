# Controllers

Cluster API has a number of controllers, both in the core Cluster API and the reference providers, which move the state of the cluster toward some defined desired state through the process of [controller reconciliation].

Documentation for the CAPI controllers can be found at:
- Bootstrap Provider
  - [Bootstrap](./controllers/bootstrap.md)
- ControlPlane Provider
  - [ControlPlane](./controllers/control-plane.md)
- Core 
  - [Cluster](./controllers/cluster.md)
  - [Machine](./controllers/machine.md)
  - [MachineSet](./controllers/machine-set.md)
  - [MachineDeployment](./controllers/machine-deployment.md)
  - [MachineHealthCheck](./controllers/machine-health-check.md)
  - [MachinePool](./controllers/machine-pool.md)


<!-- links -->
[controller reconciliation]: ../providers/implementers-guide/controllers_and_reconciliation.md