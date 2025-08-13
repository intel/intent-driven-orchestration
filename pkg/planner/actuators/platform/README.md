
# Platform feature enabling actuators

This directory contains the actuators for enabling platform features on the fly.

## Intel Resource Director Technology

Intel's [Resources Director Technology]() (RDT) enables control over how shared resources such as last-level cache (LLC)
and memory bandwidth are used by applications. This actuator enables associating the right Class of Service (COS) with
a POD. Through the COS we can control how much of the shared resources are attribute to the application. Whereby this
actuator tries to associate the COS with minimal resource capacity needs to meet the intent's targeted objectives. 

Enabling platform features on the fly for application owners showcases the ease-of-use and lifts the burden the user of 
requiring too much context/domain knowledge. While it benefits the resource owner to leverage all of its platform 
capabilities while offering better quality of service.

This actuator will set a POD level annotation that then a node-level agent can make use of & enact upon. Both 
[containerd](https://github.com/containerd/containerd) and [cri-o](https://github.com/cri-o/cri-o) offer support for 
enabling Intel RDT, and can be used for this purpose.

The actuator currently only support PODs that are in a guaranteed QoS class; as rightsizing is key to benefit from RDT.
