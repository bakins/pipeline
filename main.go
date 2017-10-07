package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"sort"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig"
	"github.com/drone/envsubst"
	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "pipeline",
	Short: "experimental command pipeline", SilenceUsage: true,
	RunE: runRootCmd,
}

func main() {
	log.SetFlags(0)
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func runRootCmd(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return errors.New("pipeline file is required")
	}

	stdOut := cmd.OutOrStdout()
	stdErr := cmd.OutOrStderr()

	p, err := loadPipelineFile(args[0])
	if err != nil {
		return errors.Wrap(err, "failed to parse pipeline file")
	}

	for _, s := range p.Steps {
		if err := s.Execute(stdOut, stdErr); err != nil {
			return errors.Wrapf(err, "step %s failed", s.Name)
		}
	}
	return nil
}

// Step is a single step
type Step struct {
	Name    string            `json:"name"`
	Type    string            `json:"type"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

// TODO: inject some standard environment vars?
func makeEnv(in map[string]string) []string {
	out := []string{}
	for k, v := range in {
		out = append(out, fmt.Sprintf("%s=%s", k, v))
	}
	sort.Strings(out)
	return out
}

// Execute will run the step
func (s *Step) Execute(stdOut, stdErr io.Writer) error {
	cmd := exec.Command(s.Command, s.Args...)
	cmd.Env = makeEnv(s.Env)
	//fmt.Println(cmd.Env)
	//fmt.Println(s.Command)
	//fmt.Println(s.Args)
	cmd.Stdout = stdOut
	cmd.Stderr = stdErr
	return cmd.Run()
}

// Pipeline represents an execution pipleine
type Pipeline struct {
	Steps []Step `json:"steps"`
}

func getEnv() map[string]string {
	environ := map[string]string{}

	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			environ[parts[0]] = parts[1]
		}
	}
	return environ
}

type rawPipeline struct {
	Steps []map[string]interface{} `json:"steps"`
}

func getName(input map[string]interface{}) (string, error) {
	v, ok := input["name"]
	if !ok {
		return "", nil
	}
	name, ok := v.(string)
	if !ok {
		return "", errors.New("invalid type for name")
	}
	return name, nil
}

func getType(input map[string]interface{}) (string, error) {
	v, ok := input["type"]
	if !ok {
		return "", nil
	}
	t, ok := v.(string)
	if !ok {
		return "", errors.New("invalid type for type")
	}
	return t, nil
}

// TODO ability to read from bytes, reader, etc
func loadPipelineFile(filename string) (*Pipeline, error) {
	tmpl, err := envsubst.ParseFile(filename)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse %s", filename)
	}

	environ := getEnv()

	data, err := tmpl.Execute(func(name string) string {
		return environ[name]
	})

	if err != nil {
		return nil, errors.Wrapf(err, "failed to execute %s", filename)
	}

	t, err := template.New("pipeline").Funcs(funcMap()).Parse(data)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse %s", filename)
	}

	buf := bytes.Buffer{}
	if err := t.ExecuteTemplate(&buf, "pipeline", nil); err != nil {
		return nil, errors.Wrapf(err, "failed to execute %s", filename)
	}

	fmt.Println(buf.String())

	r := rawPipeline{}
	if err := yaml.Unmarshal(buf.Bytes(), &r); err != nil {
		return nil, errors.Wrapf(err, "failed to parse file %s", filename)
	}

	p := &Pipeline{}
	for i, step := range r.Steps {
		name, err := getName(step)
		if err != nil {
			return nil, errors.Wrapf(err, "step %d: failed to get name", i)
		}
		if name == "" {
			name = fmt.Sprintf("step-%d", i)
		}
		t, err := getType(step)
		if err != nil {
			return nil, errors.Wrapf(err, "step %d: failed to get type", i)
		}
		if t == "" {
			t = "command"
		}
		parser := getParser(t)
		if parser == nil {
			return nil, errors.Wrapf(err, "step %d: unable to find parser for %s", i, t)
		}
		s, err := parser(step)
		if err != nil {
			return nil, errors.Wrapf(err, "step %s: failed to parse", name)
		}

		s.Name = name
		s.Type = t

		p.Steps = append(p.Steps, *s)
	}
	return p, nil
}

// text/template func map
func funcMap() template.FuncMap {
	f := sprig.TxtFuncMap()
	delete(f, "env")
	delete(f, "expandenv")
	return f
}
