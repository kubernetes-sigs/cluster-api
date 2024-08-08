<!-- DOCTOC SKIP -->
   // Return early and set the paused condition to True if the object or Cluster
   // is paused.
   // We assume that the change to the object has to be written, e.g. via the
   // patchHelper in defer.
   if annotations.IsPaused(cluster, m) {
    log.Info("Reconciliation is paused for this object")
   
    newPausedCondition := &clusterv1.Condition{
        Type:     clusterv1.PausedCondition,
        Status:   corev1.ConditionTrue,
        Severity: clusterv1.ConditionSeverityInfo,
    }
   
    if cluster.Spec.Paused {
        newPausedCondition.Reason = clusterv1.ClusterPausedReason
        newPausedCondition.Message = fmt.Sprintf("The cluster %s is paused, pausing this object until the cluster is unpaused", cluster.Name)
    } else {
        newPausedCondition.Reason = clusterv1.AnnotationPausedReason
        newPausedCondition.Message = fmt.Sprintf("The machine %s is paused, pausing this object until the annotation is removed", m.Name)
   
    }
   
    conditions.Set(m, newPausedCondition)
    return ctrl.Result{}, nil
   }
   
   conditions.MarkFalseWithNegativePolarity(m, clusterv1.PausedCondition)
