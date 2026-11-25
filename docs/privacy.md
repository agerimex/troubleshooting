# Privacy: what is sent and stored

Troubleshooting stores what applications send. Spans and log records often
carry more than they should: SQL arguments (login names, password hashes,
personal data), tokens in request URIs, user and password in URLs. Three
layers keep them out.

## 1. log-sender redacts before export (on by default)

Before a span or log record leaves the application, log-sender
(`log-sender/redact.go`):

- drops SQL arguments (`db.sql.args.*`, `db.query.parameter.*`);
- removes user and password from URLs (`scheme://user:pass@host` becomes
  `scheme://host`) in attribute **keys and values**, span names, status
  messages, event names and log messages;
- removes the query string from `http.target`, `http.url` and `url.full`,
  and drops `url.query`;
- drops the keys named by `WithRedactKeys(...)`.

A span or record that lost something lists the keys in
`troubleshooting.redacted`, so a reader knows a value was removed rather than
never recorded.

`WithSQLArgs()` keeps SQL arguments, for an application whose arguments are
not personal data (Troubleshooting's own API uses it, so its UI can show its
queries readably). log-sender then logs once at startup
"troubleshooting: SQL arguments are sent to <address>" and marks the resource
with `troubleshooting.sql_args=sent`.

`WithProcessResources()` (off by default) sends the host name, the process
id and memory and CPU figures with every span: no personal data, but the host
name names the machine (in a container, the container id).

This changes what earlier log-sender versions sent: SQL arguments and query
strings are no longer sent unless asked for.

## 2. The receiver applies the same rules to every sender

Plain OpenTelemetry senders (OTLP) don't run log-sender's step, so the
receiver applies the same rules before it stores anything
(`log-receiver/internal/data/redact.go`). It keeps SQL arguments only from a
sender whose resource has `troubleshooting.sql_args=sent`.
`TROUBLESHOOTING_REDACT=off` stores everything as sent.

## 3. The privacy scan finds what got through

```
troubleshooting-check privacy --url http://localhost:8095 --service orders-api   # or --run <run id>
```

It scans the stored spans and logs of a service or run (the last 24 h, or
`--since`, `--from`/`--to`) for SQL arguments from a sender that didn't opt
in, URL credentials (in keys too), tokens, keys, passwords and secrets in
query strings, bearer tokens and bcrypt or argon2 hashes. It prints each kind
with its count and one example span, **never the value**, and exits 0 when
nothing was found, 1 when something was, 3 on an error. API:
`POST /api/v1/privacy/scan` `{run, service, time_from, time_to}` →
`data.Privacy`: `spans`, `logs`, `findings` (`kind`, `source`, `records`,
`trace_id`, `span_id`, `what`).

The scan knows these patterns only. Names, emails or other personal data in
an application's own attributes need a review by someone who knows the
application.

## Not done yet

Removing data that was stored before these layers existed (a purge command)
is planned; until then it needs an `ALTER TABLE … DELETE` on the raw tables.
