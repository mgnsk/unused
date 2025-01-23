/*
The unused command reports unused constants, variables, types and functions in a go module.

Example:

`$ unused -generated -exclude-glob '*.pb.go'`

The tool can instructed to skip checking usage of objects by using line comments with a
`// unused:skip` prefix on the same or previous line where the unused object is defined.

The `// unused:disable` comment disables the check after the comment in the current file.

Usage: `unused [flags]`
*/
package main
