# sqsjfr

SQS job feeder.

sqsjfr reads a crontab (from local file, http URL, or S3 URL) and send job messages by the crontab schedules to Amazon SQS FIFO queue.

sqsjfr is designed to cooperate with [sqsjkr](https://github.com/kayac/sqsjkr), but runs as a standalone daemon to send messages to SQS.

## Installation

### binary packages

[Releases](https://github.com/kayac/sqsjfr/releases).

### Homebrew tap

```console
$ brew install kayac/tap/sqsjfr
```

## Usage

```
$ sqsjfr [options] (/path/to|http://...|s3://...)/crontab

Usage of sqsjfr:
  -check-interval duration
        interval of checking for crontab modified (default 1m0s)
  -dry-run
        dry run
  -log-level string
        log level (default "info")
  -message-template string
        SQS message template(JSON)
  -queue-url string
        SQS queue URL
  -stats-port int
        stats HTTP server port (default 8061)
```

Environment variables `SQSJFR_*` are also specify that options. For example, `SQSJFR_QUEUE_URL=https://sqs.ap-northeast-1.amazonaws.com/123456789012/cron.fifo`


### Example

An example of crontab.

```crontab
# comment
RUNNER=/usr/local/bin/job-runner

* * * * * echo "hello world! $(date)"; sleep 10; echo "goodby world! $(date)"
* * * * * $RUNNER -- date
```

A message template JSON.

```json
{
  "command": "{{ .Command | json_escape }}",
  "invokedAt": "{{ .InvokedAt }}",
  "envs": {
    "HOME": "{{ must_env `HOME` }}",
    "RUNNER": "{{ .Env.RUNNER }}"
  }
}
```

Template syntax `{{ }}` will be expanded when SQS messages sent.

- .Command : A command line in crontab.
- .InvokedAt : Invocation UNIX time (truncated by a minute.).
- .Env : Environment variables map which defined in crontab. When a whole .Env is evaluated as a string, returns JSON string.
- must_env `FOO` : Environment variable "FOO" defined on a running sqsjfr process.

When -message-template is not specified, default SQS message generated as below.

```json
{
  "command": "$RUNNER -- date",
  "invoked_at": 1602646620,
  "entry_id": 2,
  "envs": {
    "RUNNER":"/usr/local/bin/job-runner"
  }
}
```

```console
$ sqsjfr \
    -queue-url https://sqs.ap-northeast-1.amazonaws.com/123456789012/cron.fifo \
    -message-template message.json \
    example.crontab
2020/10/13 23:03:21.087367 [info] starting up
2020/10/13 23:03:21.090266 [info] loading crontab example.crontab
2020/10/13 23:03:21.090720 [info] [entry:1] registered > * * * * * echo "hello world! $(date)"; sleep 10; echo "goodby world! $(date)"
2020/10/13 23:03:21.090742 [info] [entry:2] registered > * * * * * $RUNNER -- date
2020/10/13 23:03:21.090765 [info] defined > RUNNER=/usr/local/bin/job-runner
2020/10/13 23:03:21.090784 [info] 2 entries registered
2020/10/13 23:03:21.090791 [info] 1 environment variables defined
2020/10/13 23:03:21.090794 [info] running daemon
2020/10/13 23:04:01.085198 [info] [entry:1] invoke job {"command":"echo \"hello world! $(date)\"; sleep 10; echo \"goodby world! $(date)\"","envs":{"HOME":"/home/sqsjfr","RUNNER":"/usr/local/bin/job-runner"},"invokedAt":"1602597840"}
2020/10/13 23:04:01.849199 [info] [entry:2] invoke job {"command":"$RUNNER -- date","envs":{"HOME":"/home/sqsjfr","RUNNER":"/usr/local/bin/job-runner"},"invokedAt":"1602597840"}
```

Schedule specs are parsed by [github.com/robfig](https://github.com/robfig/cron).
  - Blank lines and leading spaces and tabs are ignored.
  - Lines whose first non-space character is a pound-sign (#) are comments, and are ignored.

## Stats HTTP server

sqsjfr runs a stats HTTP server on port `-stats-port`(defalt 8061).

The endpoint returns metrics as JSON format.

```json
{
  "entries": {
    "registered": 2
  },
  "invocations": {
    "succeeded": 12,
    "failed": 0
  }
}
```

## High Availability

sqsjfr can be deployed by multi processes for high availability deployment.

sqsjfr sends SQS messages with [MessageDeduplicationId](https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/using-messagededuplicationid-property.html) option for SQS FIFO queue. MessageDeduplicationId is generated by a message body and an invoked UNIX timestamp(truncated by a minute).

Therefore even if multi sqsjfr processes send the same messages(has the same body and timestamp) at the same time, FIFO queue delivers one message to consumers.


## LICENSE

MIT
