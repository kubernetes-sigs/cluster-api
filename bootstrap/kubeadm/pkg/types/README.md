# Kubeadm types

These types were copied in from `kubernetes/kubernetes`.

The types found in `kubernetes/kubernetes` are incompatible with `controller-gen@v0.2`.

`controller-gen@v0.2` requires that all fields of all embedded types have json struct tags and kubeadm types are missing a few.

If the kubeadm types ever escape `kubernetes/kubernetes` then we will adopt those assuming the types do all have json struct tags.
