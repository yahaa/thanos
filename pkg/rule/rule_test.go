package thanosrule

import (
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/prometheus/prometheus/pkg/rulefmt"
	"github.com/prometheus/prometheus/rules"
	"github.com/thanos-io/thanos/pkg/store/storepb"
	"github.com/thanos-io/thanos/pkg/testutil"
	yaml "gopkg.in/yaml.v2"
)

func TestUpdate(t *testing.T) {
	dir, err := ioutil.TempDir("", "test_rule_rule_groups")
	testutil.Ok(t, err)
	defer func() { testutil.Ok(t, os.RemoveAll(dir)) }()

	testutil.Ok(t, ioutil.WriteFile(path.Join(dir, "no_strategy.yaml"), []byte(`
groups:
- name: "something1"
  rules:
  - alert: "some"
    expr: "up"
`), os.ModePerm))
	testutil.Ok(t, ioutil.WriteFile(path.Join(dir, "abort.yaml"), []byte(`
groups:
- name: "something2"
  partial_response_strategy: "abort"
  rules:
  - alert: "some"
    expr: "up"
`), os.ModePerm))
	testutil.Ok(t, ioutil.WriteFile(path.Join(dir, "warn.yaml"), []byte(`
groups:
- name: "something3"
  partial_response_strategy: "warn"
  rules:
  - alert: "some"
    expr: "up"
`), os.ModePerm))
	testutil.Ok(t, ioutil.WriteFile(path.Join(dir, "wrong.yaml"), []byte(`
groups:
- name: "something4"
  partial_response_strategy: "afafsdgsdgs" # Err 1
  rules:
  - alert: "some"
    expr: "up"
`), os.ModePerm))
	testutil.Ok(t, ioutil.WriteFile(path.Join(dir, "combined.yaml"), []byte(`
groups:
- name: "something5"
  partial_response_strategy: "warn"
  rules:
  - alert: "some"
    expr: "up"
- name: "something6"
  partial_response_strategy: "abort"
  rules:
  - alert: "some"
    expr: "up"
- name: "something7"
  rules:
  - alert: "some"
    expr: "up"
`), os.ModePerm))

	opts := rules.ManagerOptions{
		Logger: log.NewLogfmtLogger(os.Stderr),
	}
	m := NewManager(dir)
	m.SetRuleManager(storepb.PartialResponseStrategy_ABORT, rules.NewManager(&opts))
	m.SetRuleManager(storepb.PartialResponseStrategy_WARN, rules.NewManager(&opts))

	err = m.Update(10*time.Second, []string{
		path.Join(dir, "no_strategy.yaml"),
		path.Join(dir, "abort.yaml"),
		path.Join(dir, "warn.yaml"),
		path.Join(dir, "wrong.yaml"),
		path.Join(dir, "combined.yaml"),
		path.Join(dir, "non_existing.yaml"),
	})

	testutil.NotOk(t, err)
	testutil.Assert(t, strings.Contains(err.Error(), "wrong.yaml: failed to unmarshal 'partial_response_strategy'"), err.Error())
	testutil.Assert(t, strings.Contains(err.Error(), "non_existing.yaml: no such file or directory"), err.Error())

	g := m.RuleGroups()
	sort.Slice(g, func(i, j int) bool {
		return g[i].Name() < g[j].Name()
	})

	exp := []struct {
		name     string
		file     string
		strategy storepb.PartialResponseStrategy
	}{
		{
			name:     "something1",
			file:     filepath.Join(dir, "no_strategy.yaml"),
			strategy: storepb.PartialResponseStrategy_ABORT,
		},
		{
			name:     "something2",
			file:     filepath.Join(dir, "abort.yaml"),
			strategy: storepb.PartialResponseStrategy_ABORT,
		},
		{
			name:     "something3",
			file:     filepath.Join(dir, "warn.yaml"),
			strategy: storepb.PartialResponseStrategy_WARN,
		},
		{
			name:     "something5",
			file:     filepath.Join(dir, "combined.yaml"),
			strategy: storepb.PartialResponseStrategy_WARN,
		},
		{
			name:     "something6",
			file:     filepath.Join(dir, "combined.yaml"),
			strategy: storepb.PartialResponseStrategy_ABORT,
		},
		{
			name:     "something7",
			file:     filepath.Join(dir, "combined.yaml"),
			strategy: storepb.PartialResponseStrategy_ABORT,
		},
	}
	testutil.Equals(t, len(exp), len(g))
	for i := range exp {
		t.Run(exp[i].name, func(t *testing.T) {
			testutil.Equals(t, exp[i].strategy, g[i].PartialResponseStrategy)
			testutil.Equals(t, exp[i].name, g[i].Name())
			testutil.Equals(t, exp[i].file, g[i].OriginalFile())
		})
	}
}

func TestRuleGroupMarshalYAML(t *testing.T) {
	const expected = `groups:
- name: something1
  rules:
  - alert: some
    expr: up
- name: something2
  rules:
  - alert: some
    expr: up
  partial_response_strategy: ABORT
`

	a := storepb.PartialResponseStrategy_ABORT
	var input = RuleGroups{
		Groups: []RuleGroup{
			{
				RuleGroup: rulefmt.RuleGroup{
					Name: "something1",
					Rules: []rulefmt.Rule{
						{
							Alert: "some",
							Expr:  "up",
						},
					},
				},
			},
			{
				RuleGroup: rulefmt.RuleGroup{
					Name: "something2",
					Rules: []rulefmt.Rule{
						{
							Alert: "some",
							Expr:  "up",
						},
					},
				},
				PartialResponseStrategy: &a,
			},
		},
	}

	b, err := yaml.Marshal(input)
	testutil.Ok(t, err)

	testutil.Equals(t, expected, string(b))
}
