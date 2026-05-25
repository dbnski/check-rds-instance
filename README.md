# check-rds-instance

A Nagios-compatible plugin that checks any field returned by the AWS
`DescribeDBInstances` API against user-supplied expressions, including nested
objects and arrays.

## Requirements

- Go 1.21+
- AWS credentials with `rds:DescribeDBInstances` permission

## Usage

```
check-rds-instance -i <instance-id> [-e <global-expr>]
                   ( -w <warning-expr> | -c <critical-expr> | both )
```

| Flag | Required | Description |
|------|----------|-------------|
| `-i` | yes | DB instance identifier |
| `-e` | no | Global expression evaluated first; its result is available as `Value` in `-w` and `-c` |
| `-w` | at least one | Warning expression — must evaluate to a boolean |
| `-c` | at least one | Critical expression — must evaluate to a boolean |

When both `-w` and `-c` are given, critical is checked first. If neither
threshold is breached the plugin exits OK.

Exit codes follow the Nagios convention: 0 OK, 1 WARNING, 2 CRITICAL, 3 UNKNOWN.

## Expressions

Expressions are evaluated against the `DBInstance` object returned by
`DescribeDBInstances`. Field names match the Go/AWS API names exactly
(e.g. `AllocatedStorage`, `MaxAllocatedStorage`, `DBSubnetGroup`).

### Types

| Literal | Example |
|---------|---------|
| Number | `100`, `0.8` |
| String | `"Active"`, `'Active'` |
| Boolean | `true`, `false` |
| Null | `null` |

Both JSON `null` field values and fields absent from the API response compare
equal to `null`.  Only `==` and `!=` are supported for null operands; ordering
operators (`<`, `>`, etc.) return an error.

### Operators

| Category | Operators |
|----------|-----------|
| Arithmetic | `+` `-` `*` `/` |
| Comparison | `==` `!=` `<>` `<` `>` `<=` `>=` |
| Logical | `&&` `\|\|` `!` |

Standard operator precedence applies. Use parentheses to override.

`<>` is accepted as an alias for `!=` (useful in Nagios configs where `!` must be escaped).

### Field access

```
AllocatedStorage
DBSubnetGroup.VpcId
DBSubnetGroup.Subnets[0].SubnetStatus
DBParameterGroups[0].ParameterApplyStatus
```

Accessing a missing key or an out-of-bounds index returns an internal
*undefined* value rather than an error, which is the intended input for
`exists()`.

### Built-in functions

#### `exists(path)`

Returns `true` if the path resolves to a non-null value, `false` otherwise.

```
exists(DBSubnetGroup.Subnets[1])
```

#### `len(array_or_string)`

Returns the number of elements in an array, or the byte length of a string.
Returns `0` for null or undefined values.

```
len(DBSubnetGroup.Subnets)
len(DBParameterGroups)
```

#### `now()`

Returns the current UTC time. Used as a reference point for date comparisons.

```
now()
```

#### `parseTime(str)`

Parses an RFC 3339 string into a time value.

```
parseTime(ValidTill) > now()
```

String fields are automatically coerced to time values when compared against
a `time.Time` operand, so an explicit `parseTime()` wrap is only needed when
neither side is already a time value or for code clarity.

#### `INTERVAL expr UNIT`

Produces a time offset that can be added to or subtracted from a `time.Time`
value.  `expr` is any numeric expression.

| Unit | Accepted spellings |
|------|--------------------|
| Year | `YEAR`, `YEARS` |
| Month | `MONTH`, `MONTHS` |
| Week | `WEEK`, `WEEKS` |
| Day | `DAY`, `DAYS` |
| Hour | `HOUR`, `HOURS` |
| Minute | `MINUTE`, `MINUTES` |
| Second | `SECOND`, `SECONDS` |

Unit keywords are case-insensitive.  Month and year offsets use calendar
arithmetic (`AddDate`); smaller units use exact durations.

```
now() + INTERVAL 30 DAYS
now() - INTERVAL 1 YEAR
ValidTill < now() + INTERVAL 1 MONTH
```

#### `ifnull(expr, default)`

Returns `expr` if it is non-null, otherwise evaluates and returns `default`.
Both JSON `null` field values and missing/undefined fields are treated as null.

```
ifnull(ReadReplicaSourceDBInstanceIdentifier, "")
```

#### `contains(array, value)`

Returns `true` if `value` is present in `array`, `false` otherwise.

```
contains(EnabledCloudwatchLogsExports, "slowquery")
```

Compose with `&&` to require a minimum set of values:

```
contains(EnabledCloudwatchLogsExports, "error") && contains(EnabledCloudwatchLogsExports, "slowquery")
```

#### `all(array, expr)`, `any(array, expr)`, `count(array, expr)`

Iterate over an array and evaluate `expr` for each element. Inside `expr` the
current element is bound to the variable `it`.

| Function | Returns |
|----------|---------|
| `all`    | `true` if `expr` is truthy for **every** element (empty array → `false`) |
| `any`    | `true` if `expr` is truthy for **at least one** element |
| `count`  | number of elements for which `expr` is truthy |

```
all(DBParameterGroups, it.ParameterApplyStatus == "in-sync")
any(DBSubnetGroup.Subnets, it.SubnetStatus == "Active")
count(DBSubnetGroup.Subnets, it.SubnetStatus == "Active")
```

### The `Value` variable

When `-e` is specified, its result is computed before `-w`/`-c` and injected
into the expression context as `Value`. This avoids repeating a complex
sub-expression in both thresholds.

```sh
-e "AllocatedStorage / ifnull(MaxAllocatedStorage, AllocatedStorage)" \
-w "MaxAllocatedStorage != null && Value > 0.8" \
-c "MaxAllocatedStorage != null && Value > 0.9"
```

## Examples

### Storage utilisation

Warn at 80 %, alert at 90 % of the auto-scaling ceiling:

```sh
AWS_DEFAULT_REGION=eu-west-1 check-rds-instance -i my-mysql \
  -e "AllocatedStorage / ifnull(MaxAllocatedStorage, AllocatedStorage)" \
  -w "MaxAllocatedStorage != null && Value > 0.8" \
  -c "MaxAllocatedStorage != null && Value > 0.9"
```

```
OK: check passed (Value=0.6)
WARNING: Value > 0.8 (Value=0.85)
CRITICAL: Value > 0.9 (Value=0.94)
```

### Direct threshold without global expression

```sh
AWS_DEFAULT_REGION=eu-west-1 check-rds-instance -i my-mysql \
  -w "ifnull(MaxAllocatedStorage, 0) * 0.8 < AllocatedStorage" \
  -c "ifnull(MaxAllocatedStorage, 0) * 0.9 < AllocatedStorage"
```

### Parameter group sync

Alert if any parameter group is not in-sync (pending reboot):

```sh
AWS_DEFAULT_REGION=eu-west-1 check-rds-instance -i my-mysql \
  -c "any(DBParameterGroups, it.ParameterApplyStatus != \"in-sync\")"
```

Or using direct index access for a single-group setup:

```sh
AWS_DEFAULT_REGION=eu-west-1 check-rds-instance -i my-mysql \
  -c "DBParameterGroups[0].ParameterApplyStatus != \"in-sync\""
```

### Certificate / token expiry

Warn if a credential or certificate field expires within 30 days, critical if
already expired:

```sh
AWS_DEFAULT_REGION=eu-west-1 check-rds-instance -i my-mysql \
  -c "ValidTill < now() + INTERVAL 7 DAYS" \
  -w "ValidTill < now() + INTERVAL 30 DAYS"
```

### Multi-AZ enforcement

```sh
AWS_DEFAULT_REGION=eu-west-1 check-rds-instance -i my-mysql \
  -c "MultiAZ == false"
```

## AWS credentials and region

The plugin relies entirely on the standard AWS SDK environment, so no AWS flags are needed.

**Region** is resolved in order:

1. `AWS_REGION` environment variable
2. Region set in `~/.aws/config`

**Credentials** are resolved in order:

1. `AWS_ACCESS_KEY_ID` + `AWS_SECRET_ACCESS_KEY` (+ optional `AWS_SESSION_TOKEN`)
2. `AWS_SHARED_CREDENTIALS_FILE` or `~/.aws/credentials`
3. EC2/ECS instance or task IAM role

Examples:

```sh
# Explicit region and key-based credentials
AWS_REGION=eu-west-1 \
AWS_ACCESS_KEY_ID=AKIA... \
AWS_SECRET_ACCESS_KEY=... \
  check-rds-instance -i my-mysql ...

# Non-default credentials file
AWS_REGION=eu-west-1 \
AWS_SHARED_CREDENTIALS_FILE=/etc/nagios/aws-credentials \
  check-rds-instance -i my-mysql ...

# Named profile
AWS_REGION=eu-west-1 AWS_PROFILE=monitoring \
  check-rds-instance -i my-mysql ...
```

## Available fields

Any field from the
[DescribeDBInstances response](https://docs.aws.amazon.com/AmazonRDS/latest/APIReference/API_DBInstance.html)
can be referenced by its exact API name. Commonly used fields:

| Field | Type | Description |
|-------|------|-------------|
| `AllocatedStorage` | number | Current storage in GiB |
| `MaxAllocatedStorage` | number | Auto-scaling ceiling in GiB |
| `DBInstanceStatus` | string | e.g. `available`, `modifying` |
| `MultiAZ` | bool | Whether Multi-AZ is enabled |
| `EngineVersion` | string | Engine version string |
| `DBParameterGroups` | array | Parameter group assignments |
| `StorageEncrypted` | bool | Whether storage is encrypted at rest |
| `DeletionProtection` | bool | Whether deletion protection is on |
| `BackupRetentionPeriod` | number | Automated backup retention in days |