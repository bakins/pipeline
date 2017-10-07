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
			return errors.Wrapf(err, "step %s failed", s.name)
		}
	}
	return nil
}

// Step is a single step
// should there be an internal version of this?
type Step struct {
	Command    string            `json:"command"`
	Args       []string          `json:"args"`
	Env        map[string]string `json:"env"`
	name       string
	stepType   string
	conditions map[string]string
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
	// should have a seperate check/func for this
	for k, v := range s.conditions {
		// do we care about empty vs not set?
		val, ok := s.Env[k]
		if !ok {
			fmt.Printf("env var %s is not set\n", k)
			return nil
		}
		if v != val {
			fmt.Printf("env var %s: %q != %q\n", k, v, val)
			return nil
		}
	}

	cmd := exec.Command(s.Command, s.Args...)
	cmd.Env = makeEnv(s.Env)
	//fmt.Println(cmd.Env)
	fmt.Println(s.Command)
	fmt.Println(s.Args)
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

func getConditions(input map[string]interface{}) (map[string]string, error) {
	v, ok := input["when"]
	if !ok {
		return nil, nil
	}
	raw, ok := v.(map[string]interface{})
	if !ok {
		return nil, errors.New("invalid type for when")
	}
	when := map[string]string{}
	for k, v := range raw {
		val, ok := v.(string)
		if !ok {
			return nil, errors.Errorf("invalid type for key %s", k)
		}
		when[k] = val
	}
	return when, nil
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

		w, err := getConditions(step)
		if err != nil {
			return nil, errors.Wrapf(err, "step %d: failed to get when", i)
		}

		parser := getParser(t)
		if parser == nil {
			return nil, errors.Wrapf(err, "step %d: unable to find parser for %s", i, t)
		}
		s, err := parser(step)
		if err != nil {
			return nil, errors.Wrapf(err, "step %s: failed to parse", name)
		}

		s.name = name
		s.stepType = t
		s.conditions = w

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
