package command

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type generateTask struct {
	// Files are a list of JSON documents.
	Files []string `mapstructure:"files"`

	base
}

func generateTaskFactory() Command   { return &generateTask{} }
func (c *generateTask) Name() string { return "generate.tasks" }

func (c *generateTask) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, c); err != nil {
		return errors.Wrapf(err, "Error decoding %s params", c.Name())
	}
	if len(c.Files) == 0 {
		return errors.Errorf("Must provide at least 1 file to '%s'", c.Name())
	}
	return nil
}

func (c *generateTask) Execute(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {
	catcher := grip.NewBasicCatcher()
	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}
	post := [][]byte{}
	for _, fn := range c.Files {
		if ctx.Err() != nil {
			catcher.Add(ctx.Err())
			break
		}
		data, err := generateTaskForFile(fn, conf)
		if err != nil {
			catcher.Add(err)
			continue
		}
		post = append(post, data)
	}
	if catcher.HasErrors() {
		return errors.WithStack(catcher.Resolve())
	}
	return errors.Wrap(comm.GenerateTasks(ctx, td, post), "Problem posting task data")
}

func generateTaskForFile(fn string, conf *model.TaskConfig) ([]byte, error) {
	fileLoc := filepath.Join(conf.WorkDir, fn)
	if _, err := os.Stat(fileLoc); os.IsNotExist(err) {
		return nil, errors.Wrapf(err, "File '%s' does not exist", fn)
	}
	jsonFile, err := os.Open(fileLoc)
	if err != nil {
		return nil, errors.Wrapf(err, "Couldn't open file '%s'", fn)
	}
	defer jsonFile.Close()

	var data []byte
	data, err = ioutil.ReadAll(jsonFile)
	if err != nil {
		return nil, errors.Wrapf(err, "Problem reading from file '%s'", fn)
	}

	return data, nil
}
