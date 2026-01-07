(audit-logs)=
# Audit logs

JIMM provides audit logging functionality, tracking all requests/responses into the system.
This gives administrators of JIMM the ability to audit changes at a very granular level.

All requests to controllers and models are logged and can enable an analysis into why the state
of the underlying Juju estate has changed.

Audit logs are currently enabled by default without any config to disable their collection.
The logging retention period is configurable and allows for audit logs to be stored for a
variable amount of time depending on your requirements and storage constraints.

Filtering audit logs is also possible along a variety of fields including but not limited to the
user executing the request, model name and command.

Finally, access to audit logs is, by default, only afforded to JIMM superusers i.e. administrators of JIMM.
Read access to audit logs can be granted to other users, see {ref}`manage-groups`.

## Filter Logs

Querying for audit logs is most readily done via the `jaas` plugin.

### Basics

To list all available filtering options:

    juju audit-events --help

To return a list of audit logs:

    juju audit-events

The results can be filtered and paginated as described below.

Each audit log contains the following information:

- `time` [`time`]:               When the audit log was created.
- `conversation-id` [`string`]:  A unique id per Websocket connection.
- `message-id` [`uint64`]:       An incremental count for each message sent during a session.
- `facade-name` [`string`]:      The name of the grouping of methods a call is intended for, a facade categorises methods.
- `facade-method` [`string`]:    The name of the method called.
- `facade-version` [`int`]:      The version of the facade called, different client versions may use different facade version.
- `object-id` [`string`]:        A parameter used in some methods, indicates the object being acted upon.
- `user-tag` [`string`]:         The user making the request.
- `model` [`string`]:            The model a request is acting on, can be empty for controller level requests.
- `is-response` [`boolean`]:     Indicates whether this log is for a request or response message.
- `params` [`map[string]`]:      Populated during requests with any request parameters.
- `errors` [`map[string]`]:      Populated during responses with any response errors.

It's important to note that JIMM logs requests and responses separately and understanding
how to associate a request with a response is a useful tool. This can be done using the `conversation-id` and `message-id` fields.
When a client establishes a connection with JIMM and begins making requests, a unique `conversation-id` is generated for
the lifetime of that Websocket connection and each request/response pair will have the same `message-id`, which itself will
increment whenever a new request is made. Then observing the `is-response` and `errors` fields, one can ascertain whether
a call was successful.

### Example Audit Logs

An example of a request and response audit log are below.
The first is a request for a model status and the second is the response.
Note:

- The message-id is 2 because the first message (not shown) was a login request.
- The response does not include the response payload (i.e. the application status), this information is not logged.
- The request includes a "patterns" parameter that is set to null, this is specific to the `status` call.
- The facade information is only included on the request.

```text

    - time: 2023-09-06T10:21:16.72Z
    conversation-id: "b501bba5508367e5"
    message-id: 2
    facade-name: Client
    facade-method: FullStatus
    facade-version: 6
    object-id: ""
    user-tag: user-alice@external
    model: controller-1/test-model
    is-response: false
    params:
        patterns: null

    - time: 2023-09-06T10:21:16.797Z
    conversation-id: "b501bba5508367e5"
    message-id: 2
    facade-name: ""
    facade-method: ""
    facade-version: 0
    object-id: ""
    user-tag: user-alicen@external
    model: controller-1/test-model
    is-response: true
    params: {}
    errors:
        results:
        - error:
            code: ""
            message: ""
```

###  Pagination

Navigate through paginated result sets.

```text
--offset
    offset the set of returned audit events
--limit
    limit the maximum number of returned audit events
```

Audit logs are returned in a paginated set, by default, of size 50.
The size of each page can be increased to a maximum of 1000 using the
`--limit` flag.

Paging through the result set is also possible with the `--offset` flag.

For example:

    juju audit-events --offset 50

To change the page size to 100 and get the third page:

    juju audit-events --offset <(page_number-1)*100> --limit 100

### Time filters

Filter logs by time.

```
--after
    display events that happened after specified time
--before
    display events that happened before specified time
```

Note that all time values must be formatted according to RFC 3339 e.g `2023-01-02T15:04:05Z`.
Where the trailing `Z` indicates UTC time. To provide a timezone offset
one can do the following `2023-12-19T16:39:57-08:00`

Examples:

    juju audit-events --before 2023-10-12T07:20:50.52Z
    juju audit-events --before 2023-10-12T07:20:50.52-08:00
    Logs within 1 day
    juju audit-events --before 2023-10-12T07:20:50.52Z --after 2023-10-11T07:20:50.52Z

###  Method filter

Filter logs by method call.

`--method`
    display events for a specific method call

Each `juju` call invokes a specific method. This can be thought of as an HTTP handler.
Although a full list of all methods is not currently available, it is possible to filter audit events based
on the method that was called. Important methods include `Login`, `Deploy`, `DestroyApplication`, `DestroyModels`.

Note that method names are case sensitive.

Example:

    juju audit-events --method Login

### Model filter

Filter logs by the model they ran against.

`--model`
    display events for a specific model (model name is controller/model)

.. note:
    Certain methods are controller level commands, examples include `add-model` and `list-models` and are not associated with a model.
    Other commands will interact directly with a model e.g. `deploy` or `status`.

Audit logs contain information on whether a call was associated with a model and allows for filtering based on that information.

Note that the model name also needs to include the controller the model was deployed against e.g. `<controller>/<model>`
To obtain the controller a model was deployed against is currently a challenge, currently the easiest approach is to simply query the
audit logs using other filters and identify an audit log against the desired model which will contain the controller and model name.

Example:

    juju audit-events --model my-controller/model-name

###  User filter

Filter logs by the user making the request.

`--user-tag`
    display events performed by authenticated user

Almost every audit log will include the username for the user who made the call.
One notable exception is that at the start of a session, the client will perform a login call, at
which point JIMM is unaware of the user. If the login is successful, the response will include the username
of the authenticated user and so will further requests during that session.

Note that the user tag will normally be of the form user-<username>@external

Example:

    juju audit-events --user-tag user-alice@external

### Order

Change the order logs are returned.

```text
--reverse: reverse the order of logs, showing the most recent first
```

By default, audit logs are shown in chronological order with the oldest events
returned first.

Using the `reverse` flag will change the order to return the latest event in
the range first.


## Log Retention

Log retention determines how long audit logs are stored before being purged. Because audit logs are stored in JIMM's
database, the size requirements for the database will grow over time. This can be managed by automatically purging
logs older than a certain date and will vary based on auditability needs.

This can be configured on the charm using the `audit-log-retention-period-in-days` config option. As the name implies,
this will determine the audit log retention period, in days. Audit logs currently get purged at 9 AM UTC daily.
Therefore, a value of e.g. 1 implies that all logs older than 1 day, from the time the cleanup triggers, will be purged.

Because the purge happens daily at a fixed time, there is some time in which logs older than the configured retention
period will be kept, at least until the next cleanup.

## Purge Logs

It is also possible to manually purge audit-logs.

This can also be done via the CLI and again only JIMM admins have rights to purge audit logs. In this case,
other users cannot be granted this permission.

    juju purge-audit-logs <date>

This command will purge audit logs from the database before the given date.
Note that the date format is flexible, accepting both a date or date and time.

Note that omitting the date will assume zero for the time, i.e. the start of that day.

Examples:

    juju purge-audit-logs 2021-02-03
    juju purge-audit-logs 2021-02-03T15:04:05Z

