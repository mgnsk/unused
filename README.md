The unused command reports unused constants, variables, types and functions in a go module.

Example:

`$ unused -generated -exclude-files '*.pb.go' -exclude-names '*FakeService'`

The tool can instructed to skip checking usage of objects by using line comments with a
`// unused:skip` prefix on the same or previous line where the unused object is defined.

The `// unused:disable` comment disables the check after the comment in the current file.

Usage: `unused [flags]`

## Installation

`$ go install github.com/mgnsk/unused@latest`
