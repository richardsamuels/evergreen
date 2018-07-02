package plugin

import (
	"fmt"
	"html/template"
	"io/ioutil"
	"path/filepath"

	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
)

func init() {
	Publish(&PerfPlugin{})
}

var includes = []template.HTML{
	`<script type="text/javascript" src="/static/plugins/perf/js/trend_chart.js"></script>`,
	`<script type="text/javascript" src="/static/plugins/perf/js/perf.js"></script>`,
}

// PerfPlugin displays performance statistics in the UI.
type PerfPlugin struct {
	Projects []string `yaml:"string"`
}

// Name implements Plugin Interface.
func (pp *PerfPlugin) Name() string { return "perf" }

func (pp *PerfPlugin) Configure(params map[string]interface{}) error {
	err := mapstructure.Decode(params, pp)
	if err != nil {
		return fmt.Errorf("error decoding %v params: %v", pp.Name(), err)
	}
	return nil
}

func (pp *PerfPlugin) GetPanelConfig() (*PanelConfig, error) {
	panelHTML, err := ioutil.ReadFile(filepath.Join(TemplateRoot(pp.Name()), "task_perf_data.html"))
	if err != nil {
		return nil, fmt.Errorf("Can't load panel html file: %v", err)
	}

	return &PanelConfig{
		Panels: []UIPanel{
			{
				Includes:  includes,
				Page:      TaskPage,
				Position:  PageCenter,
				PanelHTML: template.HTML(panelHTML),
				DataFunc: func(context UIContext) (interface{}, error) {
					return struct {
						Enabled bool `json:"enabled"`
					}{util.StringSliceContains(pp.Projects, context.ProjectRef.Identifier)}, nil
				},
			},
		},
	}, nil
}
