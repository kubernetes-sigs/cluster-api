# MachineSet Controller

`MachineSet`s are currently different from `Cluster` and `Machine` resources in
that they do not define an actuator interface. They are generic controllers
which implement their intent by modifying provider-specific `Cluster` and 
`Machine` resources.

{% method %}
## MachineSet

{% sample lang="go" %}
[import:'MachineSet'](../../../pkg/apis/cluster/v1alpha1/machineset_types.go)
{% endmethod %}

{% method %}
## MachineSetSpec

{% sample lang="go" %}
[import:'MachineSetSpec'](../../../pkg/apis/cluster/v1alpha1/machineset_types.go)
{% endmethod %}

{% method %}
## MachineSetTemplateSpec

{% sample lang="go" %}
[import:'MachineSetTemplateSpec'](../../../pkg/apis/cluster/v1alpha1/machineset_types.go)
{% endmethod %}

{% method %}
## MachineSetStatus

{% sample lang="go" %}
[import:'MachineSetStatus'](../../../pkg/apis/cluster/v1alpha1/machineset_types.go)
{% endmethod %}
