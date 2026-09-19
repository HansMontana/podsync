# Example Configurations

These files demonstrate supported podsync configuration patterns. They are
examples for documentation and testing, not recommendations or endorsements
of the listed podcasts and news sources.

- `podcasts.toml` shows ordinary podcast sources with bounded logical-feed
  queues, ordering, and unplayed-only selection.
- `daily-briefing.toml` shows several sources partitioned into logical feeds
  and combined into one ordered briefing.

Validate either file without changing a device:

```bash
podsync validate-config -config examples/podcasts.toml
podsync validate-config -config examples/daily-briefing.toml
```

The DLF Presseschau source combines multiple editions. The example uses the
generic `title_contains = "Zeitungen"` filter to create a logical feed for the
newspaper editions. This depends on the current published title wording and
should be reviewed if DLF changes those titles.

The configuration format does not fetch feeds during validation. Refreshing a
configured device performs the network requests and stores the resulting feed
and episode state on the device.
