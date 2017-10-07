package main

import (
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

// StepParserFunc is implemented by parsers to transform a
// "plugin" step to a normal one
type StepParserFunc func(map[string]interface{}) (*Step, error)

var typesRegistry = map[string]StepParserFunc{}

// RegistryParser will register a parse. This is not thread safe.
func RegistryParser(name string, f StepParserFunc) {
	typesRegistry[name] = f
}

//
func getParser(name string) StepParserFunc {
	return typesRegistry[name]
}

// UnmarshalConfig will unmarshal a generic config into the type needed
// by the type
func UnmarshalConfig(input map[string]interface{}, val interface{}) error {
	return mapstructure.Decode(input, val)
}

func init() {
	RegistryParser("command", stepParser)
	RegistryParser("docker", dockerParser)
}

// standard type
func stepParser(input map[string]interface{}) (*Step, error) {
	s := Step{}
	if err := UnmarshalConfig(input, &s); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal step")
	}
	return &s, nil
}

// docker

type dockerStep struct {
	Env     map[string]string
	Image   string
	Command string
	Args    []string
}

func dockerParser(input map[string]interface{}) (*Step, error) {
	d := dockerStep{}
	if err := UnmarshalConfig(input, &d); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal docker step")
	}

	if d.Image == "" {
		return nil, errors.New("image is required")
	}

	s := &Step{
		Command: "docker",
		Env:     d.Env,
	}
	args := []string{"run",
		"-t",
		"--entrypoint=",
		d.Image,
		d.Command,
	}
	s.Args = append(args, d.Args...)
	return s, nil
}
