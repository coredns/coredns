# import

## Name

*import* - include files or reference snippets from a Corefile

## Description

The *import* plugin can be used to incude files into the main configuration. Another use it to
reference predefined snippets. Both can help to avoid some duplication.

This is a unique directive in that *import* can appear outside of a server block. In other words, it
can appear at the top of a Corefile where an address would normally be. Like other directives,
however, it cannot be used inside of other directives.

That the the import path is relative to the Corefile.

## Syntax

~~~
import PATTERN
~~~

* **PATTERN** is the file or glob pattern (`*`) to include. Its contents will replace this line, as
  if that file's contents appeared here to begin with. This value is relative to the file's
  location. It is an error if a specific file cannot be found, but an empty glob pattern is not an
  error.

## Examples

Import a shared configuration:

~~~
import config/common.conf
~~~

Imports any files found in the zones directory:

~~~
import ../zones/*
~~~

## Also See

See corefile(5).
