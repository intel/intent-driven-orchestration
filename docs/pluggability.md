
# Plugin Extensions

The initial framework comes with a predefined set of actuators and the A* Planner. In some cases, administrators might
want to use another set of actuators which can enable various orchestration activities. Similarly, different planning
algorithm or multiple planners for several groups of workloads can be supported. Therefore, the overall framework
supports a concept of plugins.

## Actuator Plugins

The current framework supports hot-pluggable actuators. The Intent Controller implements a component responsible for
the overall plugin management. The plugin manager exposes an endpoint that can be used by the available plugins.

As shown in the following diagram, new plugins have to register against this endpoint to be used in the actual planning
cycle:

![plugins, actuator-plugins, plugin-manager](fig/plugin_manager.png)

After successful registration, the planner will call the functions **_NextState_**, **_Perform_** and **_Effect_** via
[gRPC](https://grpc.io/). The bi-directional streaming is implemented on **_NextState_** which has demonstrated an 
improvement on the planner's performance.

## Using GRPC to launch an actuator

The [plugins_helper](../plugins/plugins_helper.go) provides a function that can be used to connect an actuator with the
rest of the framework. To do so, the user will need to provide the GRPC endpoint and port for the actuator as well as
for the plugin manager:   

    actuator := <...>
    plugins.StartActuatorPlugin(actuator, "localhost", 12345, "localhost", "33333")
