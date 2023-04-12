
# Scaling actuators

This directory contains the actuators for horizontal and vertical scaling of cloud-native applications. Both actuators 
enable control of latency related objectives for now.

## Horizontal scaling

Horizontal scaling is realized through two actions:

  1. [scale_out](scale_out.go) which can set the number of replicas for a Deployment/ReplicaSet, and
  2. [rm_pod](rm_pod.go) which can remove PODs from Deployment/ReplicaSets.

## Vertical scaling

Vertical scaling is supported for [cpu](cpu_scale.go) resources. 

If proactive planning is enabled (by providing a non-zero value for the maximum proactive CPU property) in the 
configuration, this actuator will aim to rightsize the resource allocations. If a given workload resource has no 
resource requests or limits defined and the desired SLOs are not fulfilled, the actuator will try to determine the most 
efficient settings. If resource requests and limits are defined, and the SLOs are not fulfilled, the proactive planning 
will also be active. If the targeted SLOs are reached, the proactive scaling will stop.

Once a model is available in the database - which described the most efficient resource allocations - the actuator will 
set the resources requests/limits for the PODs.

For now the actuator will tune the resource allocation for the last container in a POD.
