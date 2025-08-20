# Observability for Planner Logs

This document outlines how to set up the IDO planner config, logging utilities and observability tools in order to 
collect, export and query logs. The logging utilities (Fluent-bit and Logrotate) and the observability tools 
(OpenTelemetry Collector, Loki, and Grafana) used here are provided as examples, and the same principles should apply 
to any other similar tools. **Please note that we do not maintain or support these specific tools**. 

## 1. IDO Planner

This section explains how to set up the [IDO framework](framework.md) to save logs to a file (in addition to the
standard output), along with setting up logging utilities that handle log forwarding and log rotation, respectively.

### 1.1. Log file configuration
To enable the IDO planner saving the logs to a file, add the `log_file` attribute to the configuration file as shown 
below:

```json
  ...
  "generic": {
    "log_file": "<logfile_path>"
  }
  ...
```

Key points to consider when configuring the log file:
* **Empty String**: If the value of the `log_file` attribute is an empty string, the planner will treat it as if the log 
  file is not set at all.
* **Incorrect Path**: If the specified log file path is incorrect, the planner will throw a panic error.
* **Path Types**: Both relative and absolute paths are accepted.
* **File Creation**: If the log file path is correct but the log file does not exist, it will be created automatically.
* **File Append**: If the log file already exists, new log entries will be appended to it rather than overwriting the 
  existing file.

### 1.2. Log forwarder 
The log forwarder is responsible for collecting log data from the `log_file` produced by the IDO planner and forwarding 
it to a centralized logging system. Below is a sample configuration for [Fluent-bit](https://docs.fluentbit.io/manual)  forwarding logs to 
OpenTelemetry endpoint:   

```ini
[SERVICE]
    Flush        1
    Log_Level    info
    Parsers_File parsers.conf`
[INPUT]
    Name              tail
    Path              <logfile_path>
    Parser            docker
    Tag               kube.*
    Refresh_Interval  5
    Skip_Long_Lines   On
    DB                /var/log/flb_kube.db
    DB.Sync           Normal
[OUTPUT]
    Name  opentelemetry
    Match *
    Host  <opentelemetry_addr>
    Port  4318
    Logs_uri  /v1/logs
parsers.conf: |
[PARSER]
    Name   docker
    Format json
    Time_Key time
    Time_Format %Y-%m-%dT%H:%M:%S.%L
```

### 1.3. Log rotation
Log rotation is responsible for managing the size and lifecycle of the IDO planner's log file, ensuring it does not 
consume excessive disk space. Below is a sample configuration for [Logrotate](https://github.com/blacklabelops/logrotate), derived from the 
default [kubelet log rotate configuration](https://kubernetes.io/docs/concepts/cluster-administration/logging/#log-rotation):

```ini
<logfile_path> {
        su root root
        size 10M
        rotate 5
        compress
        copytruncate
        missingok
        notifempty
    }
```

### 1.4. Deployment considerations
To ensure all the components work correctly together: 
* The `<logfile_path>` should be consistent across the IDO planner configuration and the utilities for log forwarding 
  and log rotation.
* In a Kubernetes deployment:
    * Consider creating a shared volume to store the log file, allowing it to be mounted and accessed by each of the 
      IDO planner and the log utilities containers.
    * The utilies for log forwarding and log rotation can be deployed as sidecars within the same IDO planner Pod.  

## 2. Observability Collector  
The observability collector is responsible for receiving the logs from the log forwarder and preparing them for
ingestion into a log aggregator.  Below is a sample configuration for the [OpenTelemetry Collector](https://opentelemetry.io/docs/collector/) ingesting logs 
to the log aggregator [Loki](https://grafana.com/docs/loki/latest/send-data/otel/):

```yaml
receivers:
    otlp:
        protocols:
            http:
                endpoint: 0.0.0.0:4318 
processors:
    batch:
exporters:
    otlphttp:
        endpoint: <loki_addr>:3100/otlp
        tls:
            insecure: true
service:
    pipelines:
    logs:
        receivers: [otlp]
        processors: [batch]
        exporters: [otlphttp]
```

## 3. Log Aggregator
The log aggregator is responsible for collecting, storing, and querying log entries. Below is a sample configuration of 
the log aggregator [Loki](https://grafana.com/docs/loki/latest/): 

```yaml
auth_enabled: false
limits_config:
    allow_structured_metadata: true
    volume_enabled: true
    reject_old_samples: false 
server:
    http_listen_port: 3100
common:
    instance_addr: 0.0.0.0
    ring:
    kvstore:
        store: inmemory
    replication_factor: 1
    path_prefix: /tmp/loki
schema_config:
    configs:
    - from: 2020-05-15
    store: tsdb
    object_store: filesystem
    schema: v13
    index:
        prefix: index_
        period: 24h
storage_config:
    tsdb_shipper:
    active_index_directory: /tmp/loki/index
    cache_location: /tmp/loki/index_cache
    filesystem:
    directory: /tmp/loki/chunks
pattern_ingester:
    enabled: true
```

## 4. Log Queries
Log queries for the IDO planner can be efficiently handled using a log aggregator. For instance, with Loki, it is 
possible to perform log queries through HTTP requests or a dashboard.

### 4.1. HTTP Requests 
Loki expose [HTTP endpoints](https://grafana.com/docs/loki/latest/reference/loki-http-api/#query-endpoints) to query logs related data. For more information on the query logic and syntax, refer
to the [Loki log queries documentation](https://grafana.com/docs/loki/latest/query/log_queries/). Find below some query examples:

* Get all labels in log entries:
```sh
curl -G "http://<loki-addr>:3100/loki/api/v1/labels"
```
* Get logs filtered using a label, within a time interval:
```sh
curl -G "<loki-addr>:3100/loki/api/v1/query" --data-urlencode 'query={<label>="<value>"}' \
--data-urlencode start="<date-and-time>" \
--data-urlencode end="<date-and-time>"
```

* Get logs containing a specific string, within a time interval. In the context of IDO planner, the string can be the
  intent-name of the workload of interest:
```sh
curl -G "<loki-addr>:3100/loki/api/v1/query" --data-urlencode 'query={job=~".+"} |= "<intent-name>"' \
--data-urlencode start="<date-and-time>" \
--data-urlencode end="<date-and-time>"
```

### 4.2 Dashboard
Dashboards offer a user-friendly way to visualize and query logs. For instance,
[Grafana](https://grafana.com/docs/grafana/latest/introduction/) can easily be integrated to Loki. Below is an example of Grafana configuration to set up Loki as a
default data source: 

```yaml
apiVersion: 1
datasources:
  - name: Loki
    type: loki
    access: proxy
    url: http://<loki_addr>:3100
    isDefault: true
```

Logs can be visualized and queried in Grafana through the [simplified exploration](https://grafana.com/docs/grafana/latest/explore/simplified-exploration/logs/) or by adding the Loki data 
source to a dashboard.
