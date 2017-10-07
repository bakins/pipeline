# pipeline

Experimentation with command pipelines.

This is an experiement and nothing more.

## Usage

see [simple.yaml](./simple.yaml) for an example pipeline.

Steps are defined commands to run. "plugins" can be used to add configuration options,
but all plugins simple return commands to be ran.

To run, clone this repository and built it with `go build .` inside the repo. The run
`./pipeline ./simple.yaml`

You can play with environment substitution and templating.

pipeline first does environment substitution, then runs the output of that through
Go [text/template](https://golang.org/pkg/text/template/). Note: templating is meant for
advanced use cases.

## LICENSE

see [LICENCE](./LICENSE)