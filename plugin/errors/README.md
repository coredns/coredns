# errors

## Name

*errors* - enables error logging.

## Description

Any errors encountered during the query processing will be printed to standard output. The errors of particular type can be consolidated and printed once per some period of time.

This plugin can only be used once per Server Block.

## Syntax

The basic syntax is:

~~~
errors
~~~

Extra knobs are available with an expanded syntax:

~~~
errors {
	stacktrace
	consolidate DURATION REGEXP [LEVEL] [show_first]
}
~~~

Option `stacktrace` will log a stacktrace during panic recovery.

Option `consolidate` allows collecting several error messages matching the regular expression **REGEXP** during **DURATION**. **REGEXP** must not exceed 10000 characters. After the **DURATION** since receiving the first such message, the consolidated message will be printed to standard output with
log level, which is configurable by optional option **LEVEL**. Supported options for **LEVEL** option are `warning`,`error`,`info` and `debug`.
~~~
2 errors like '^read udp .* i/o timeout$' occurred in last 30s
~~~

If the optional `show_first` flag is specified, the first error will be logged immediately when it occurs, and then subsequent matching errors will be consolidated. When the consolidation period ends:
- If only one error occurred, no summary is printed (since it was already logged)
- If multiple errors occurred, a summary is printed showing the total count

Example with 3 errors:
~~~
[WARNING] 2 example.org. A: read udp 10.0.0.1:53->8.8.8.8:53: i/o timeout
[WARNING] 3 errors like '^read udp .* i/o timeout$' occurred in last 30s
~~~

Example with 1 error:
~~~
[WARNING] 2 example.org. A: read udp 10.0.0.1:53->8.8.8.8:53: i/o timeout
~~~

Multiple `consolidate` options with different **DURATION** and **REGEXP** are allowed. In case if some error message corresponds to several defined regular expressions the message will be associated with the first appropriate **REGEXP**.

For better performance, it's recommended to use the `^` or `$` metacharacters in regular expression when filtering error messages by prefix or suffix, e.g. `^failed to .*`, or `.* timeout$`.

## Examples

Use the *whoami* to respond to queries in the example.org domain and Log errors to standard output.

~~~ corefile
example.org {
    whoami
    errors
}
~~~

Use the *forward* plugin to resolve queries via 8.8.8.8 and print consolidated messages
for errors with suffix " i/o timeout" as warnings,
and errors with prefix "Failed to " as errors.

~~~ corefile
. {
    forward . 8.8.8.8
    errors {
        consolidate 5m ".* i/o timeout$" warning
        consolidate 30s "^Failed to .+"
    }
}
~~~

Use the *forward* plugin and consolidate timeout errors with `show_first` option to see both
the summary and the first occurrence of the error:

~~~ corefile
. {
    forward . 8.8.8.8
    errors {
        consolidate 5m ".* i/o timeout$" warning show_first
        consolidate 30s "^Failed to .+" error show_first
    }
}
~~~
