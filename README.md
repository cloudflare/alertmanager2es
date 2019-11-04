# alertmanager2es

alertmanager2es receives [HTTP webhook][] notifications from [AlertManager][]
and inserts them into an [Elasticsearch][] index for searching and analysis. It
runs as a daemon.

The alerts are stored in Elasticsearch as [alert groups][].

[alert groups]: https://prometheus.io/docs/alerting/alertmanager/#grouping
[AlertManager]: https://github.com/prometheus/alertmanager
[Elasticsearch]: https://www.elastic.co/products/elasticsearch
[HTTP webhook]: https://prometheus.io/docs/alerting/configuration/#webhook-receiver-<webhook_config>

## Rationale

It can be useful to see which alerts fired over a given time period, and
perform historical analysis of when and where alerts fired. Having this data
can help:

- tune alerting rules
- understand the impact of an incident
- understand which alerts fired during an incident

It might have been possible to configure Alertmanager to send the alert groups
to Elasticsearch directly, if not for the fact that [Elasticsearch][] [does not
support unsigned integers][] at the time of writing. Alertmanager uses an
unsigned integer for the `groupKey` field, which alertmanager2es converts to a
string.

[does not support unsigned integers]: https://github.com/elastic/elasticsearch/issues/13951

## Limitations

- alertmanager2es will not capture [silenced][] or [inhibited][] alerts; the alert
  notifications stored in Elasticsearch will closely resemble the notifications
  received by a human.

[silenced]: https://prometheus.io/docs/alerting/alertmanager/#silences
[inhibited]: https://prometheus.io/docs/alerting/alertmanager/#inhibition

- Kibana does not display arrays of objects well (the alert groupings use an
  array), so you may find some irregularities when exploring the alert data in
  Kibana. We have not found this to be a significant limitation, and it is
  possible to query alert labels stored within the array.

## Prerequisites

To use alertmanager2es, you'll need:

- an [Elasticsearch][] cluster
- [Alertmanager][] 0.6.0 or above

To build alertmanager2es, you'll need:

- [Make][]
- [Go][] 1.7 or above
- a working [GOPATH][]

[Make]: https://www.gnu.org/software/make/
[Go]: https://golang.org/dl/
[GOPATH]: https://golang.org/cmd/go/#hdr-GOPATH_environment_variable

## Building

    go get -u github.com/cloudflare/alertmanager2es
    cd $GOPATH/src/github.com/cloudflare/alertmanager2es
    make

## Configuration

### alertmanager2es usage

alertmanager2es is configured using commandline flags. It is assumed that
alertmanager2es has unrestricted access to your Elasticsearch cluster.

alertmanager2es does not perform any user authentication.

Run `./alertmanager2es -help` to view the configurable commandline flags.

### Example Alertmanager configuration

#### Receiver configuration

```yaml
- name: alertmanager2es
  webhook_configs:
    - url: https://alertmanager2es.example.com/webhook
```

#### Route configuration

By omitting a matcher, this route will match all alerts:

```yaml
- receiver: alertmanager2es
  continue: true
```

### Example Elasticsearch template

Apply this Elasticsearch template before you configure alertmanager2es to start
sending data:

```json
{
  "template": "alertmanager-2*",
  "settings": {
    "number_of_shards": 1,
    "number_of_replicas": 1,
    "index.refresh_interval": "10s",
    "index.query.default_field": "groupLabels.alertname"
  },
  "mappings": {
    "_default_": {
      "_all": {
        "enabled": false
      },
      "properties": {
        "@timestamp": {
          "type": "date",
          "doc_values": true
        }
      },
      "dynamic_templates": [
        {
          "string_fields": {
            "match": "*",
            "match_mapping_type": "string",
            "mapping": {
              "type": "keyword",
              "index": "true",
              "ignore_above": 1024,
              "doc_values": true
            }
          }
        }
      ]
    }
  }
}
```

We rotate our index once a month, since there's not enough data to warrant
daily rotation in our case. Therefore our index name looks like:

    alertmanager-200601

We anchor the template name with `-2` to avoid inadvertently matching other
indices, e.g.  `alertmanager-foo-200601`. This of course assumes that you will
no longer care to index your alerts in the year 3000.

## Failure modes

alertmanager2es will return a HTTP 500 (Internal Server Error) if it encounters
a non-2xx response from Elasticsearch. Therefore if Elasticsearch is down,
alertmanager2es will respond to Alertmanager with a HTTP 500. No retries are
made as Alertmanager has its own retry logic.

Both the HTTP server exposed by alertmanager2es and the HTTP client that
connects to Elasticsearch have read and write timeouts of 10 seconds.

## Metrics

alertmanager2es exposes [Prometheus][] metrics on `/metrics`.

[Prometheus]: https://prometheus.io/

## Example Elasticsearch queries

    alerts.labels.alertname:"Disk_Likely_To_Fill_Next_4_Days"

## Contributions

Pull requests, comments and suggestions are welcome.

Please see [CONTRIBUTING.md](CONTRIBUTING.md) for more information.
