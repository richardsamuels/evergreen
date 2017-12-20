package command

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	modelutil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/evergreen/plugin/plugintest"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/smartystreets/goconvey/convey/reporting"
	"github.com/stretchr/testify/suite"
)

type GitGetProjectSuite struct {
	suite.Suite

	modelData1 *modelutil.TestModelData // test model for TestGitPlugin
	modelData2 *modelutil.TestModelData // test model for TestValidateGitCommands
}

func init() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
	reporting.QuietMode()
}

func TestGitGetProjectSuite(t *testing.T) {
	suite.Run(t, new(GitGetProjectSuite))
}

func (s *GitGetProjectSuite) SetupTest() {
	var err error
	testConfig := testutil.TestConfig()
	s.NoError(err)
	configPath1 := filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "git", "plugin_clone.yml")
	configPath2 := filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "git", "test_config.yml")
	patchPath := filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "git", "test.patch")
	s.modelData1, err = modelutil.SetupAPITestData(testConfig, "test", "rhel55", configPath1, modelutil.NoPatch)
	s.NoError(err)

	s.modelData2, err = modelutil.SetupAPITestData(testConfig, "test", "rhel55", configPath2, modelutil.NoPatch)
	s.NoError(err)
	//SetupAPITestData always creates BuildVariant with no modules so this line works around that
	s.modelData2.TaskConfig.BuildVariant.Modules = []string{"sample"}
	err = plugintest.SetupPatchData(s.modelData1, patchPath, s.T())
	s.NoError(err)
}

func (s *GitGetProjectSuite) TestGitPlugin() {
	conf := s.modelData1.TaskConfig
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	comm := client.NewMock("http://localhost.com")
	logger := comm.GetLoggerProducer(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret})

	for _, task := range conf.Project.Tasks {
		s.NotEqual(len(task.Commands), 0)
		for _, command := range task.Commands {

			pluginCmds, err := Render(command, conf.Project.Functions)
			s.NoError(err)
			s.NotNil(pluginCmds)
			err = pluginCmds[0].Execute(ctx, comm, logger, conf)
			s.NoError(err)
		}
	}
}

func (s *GitGetProjectSuite) TestValidateGitCommands() {
	const refToCompare = "cf46076567e4949f9fc68e0634139d4ac495c89b" //note: also defined in test_config.yml

	conf := s.modelData2.TaskConfig
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	comm := client.NewMock("http://localhost.com")
	logger := comm.GetLoggerProducer(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret})

	for _, task := range conf.Project.Tasks {
		for _, command := range task.Commands {
			pluginCmds, err := Render(command, conf.Project.Functions)
			s.NoError(err)
			s.NotNil(pluginCmds)
			err = pluginCmds[0].Execute(ctx, comm, logger, conf)
			s.NoError(err)
		}
	}
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = conf.WorkDir + "/src/module/sample/"
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	s.NoError(err)
	ref := strings.Trim(out.String(), "\n") // revision that we actually checked out
	s.Equal(refToCompare, ref)
}

func (s *GitGetProjectSuite) TestBuildHTTPCloneCommand() {
	projectRef := &model.ProjectRef{
		Owner:  "deafgoat",
		Repo:   "mci_test",
		Branch: "master",
	}

	c := gitFetchProject{
		Directory: "dir",
		Token:     "token",
	}

	cmds, err := c.buildHTTPCloneCommand(projectRef)
	s.NoError(err)
	s.Len(cmds, 6)
	s.Equal("git init 'dir'", cmds[0])
	s.Equal("cd dir; git checkout -b 'master'", cmds[1])
	s.Equal("set +o xtrace", cmds[2])
	s.Equal("echo \"cd dir; git pull 'https://token:x-oauth-basic@github.com/deafgoat/mci_test.git' 'master'\"", cmds[3])
	s.Equal("cd dir; git pull 'https://token:x-oauth-basic@github.com/deafgoat/mci_test.git' 'master'", cmds[4])
	s.Equal("set -o xtrace", cmds[5])

	projectRef.Branch = ""
	cmds, err = c.buildHTTPCloneCommand(projectRef)
	s.NoError(err)
	s.Len(cmds, 5)
	s.Equal("git init 'dir'", cmds[0])
	s.Equal("set +o xtrace", cmds[1])
	s.Equal("echo \"cd dir; git pull 'https://token:x-oauth-basic@github.com/deafgoat/mci_test.git'\"", cmds[2])
	s.Equal("cd dir; git pull 'https://token:x-oauth-basic@github.com/deafgoat/mci_test.git'", cmds[3])
	s.Equal("set -o xtrace", cmds[4])

	projectRef.Owner = ""
	cmds, err = c.buildHTTPCloneCommand(projectRef)
	s.Error(err)
	s.Nil(cmds)
}

func (s *GitGetProjectSuite) TestBuildSSHCloneCommand() {
	projectRef := &model.ProjectRef{
		Owner:  "deafgoat",
		Repo:   "mci_test",
		Branch: "master",
	}

	c := gitFetchProject{
		Directory: "dir",
	}

	cmds, err := c.buildSSHCloneCommand(projectRef)
	s.NoError(err)
	s.Len(cmds, 1)
	s.Equal("git clone 'git@github.com:deafgoat/mci_test.git' 'dir' --branch 'master'", cmds[0])

	projectRef.Branch = ""
	cmds, err = c.buildSSHCloneCommand(projectRef)
	s.NoError(err)
	s.Len(cmds, 1)
	s.Equal("git clone 'git@github.com:deafgoat/mci_test.git' 'dir'", cmds[0])

	projectRef.Owner = ""
	cmds, err = c.buildSSHCloneCommand(projectRef)
	s.Error(err)
	s.Nil(cmds)
}

func (s *GitGetProjectSuite) TestBuildCommand() {
	conf := s.modelData1.TaskConfig

	c := gitFetchProject{
		Directory: "dir",
	}

	cmds, err := c.buildCloneCommand(conf)
	s.NoError(err)
	s.Len(cmds, 5)
	s.Equal("set -o xtrace", cmds[0])
	s.Equal("set -o errexit", cmds[1])
	s.Equal("rm -rf dir", cmds[2])
	s.Equal("git clone 'git@github.com:deafgoat/mci_test.git' 'dir' --branch 'master'", cmds[3])
	s.Equal("cd dir; git reset --hard ", cmds[4])

	conf.ProjectRef.Owner = ""
	cmds, err = c.buildCloneCommand(conf)
	s.Error(err)
	s.Nil(cmds)
}

func (s *GitGetProjectSuite) TearDownSuite() {
	if s.modelData1.TaskConfig != nil {
		s.NoError(os.RemoveAll(s.modelData1.TaskConfig.WorkDir))
	}
	if s.modelData2.TaskConfig != nil {
		s.NoError(os.RemoveAll(s.modelData2.TaskConfig.WorkDir))
	}
}
