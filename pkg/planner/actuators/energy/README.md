
# Energy efficient actuator

The goal of this actuator is to most efficiently set up the system in context of the carbon, power and performance
objectives of a workload. Initially, the actuator will aim to minimize power draw as much as possible.

## Config

| Property                 | Description                                                                                                                                                                                                                           |
|--------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| options                  | Ordered list of profile names; Whereby the most power efficient one is listed first, the most performant one last (currently assuming 4 profiles: None + 3 configured profiles.).                                                     |
| add_proactive_candidates | Tells the actuator to add candidate options if no proper solution can be found.                                                                                                                                                       |
| renewable_limit          | If ratio of renewable energy falls below this setting, the actuator will start throttling the deployed workloads (set to 0 to deactivate this feature). Assumes a metrics with the name _renewable_energy_ratio_ is being configured. |
| step_down                | In case a lower power drawing profile needs to be select, this defines by how many profiles the actuator will step down.                                                                                                              |
| ...                      | For other options such as endpoints etc., see similar examples [here](../../../../docs/getting_started.md)                                                                                                                            |

## Considerations

  * When used with the Kubernetes power manager, the PODs need to be in a guaranteed QoS; this requires the vertical 
    scaling actuator to be configured accordingly.
  * Currently, the actuator relies on the Kubernetes power manager to enact the power setting. Meanwhile, this project 
    has been archived, but the actuator can be adapted to support other power management tools available for Kubernetes. 
    This requires changing the _Effect()_ call accordingly.
  * Note that currently the actuator works with four profiles.
