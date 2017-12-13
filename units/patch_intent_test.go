package units

import (
	"context"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

var testConfig = testutil.TestConfig()

func init() {
	db.SetGlobalSessionProvider(testConfig.SessionFactory())
}

type PatchIntentUnitsSuite struct {
	sender *send.InternalSender
	env    *mock.Environment
	cancel context.CancelFunc

	repo     string
	headRepo string
	prNumber int
	user     string
	hash     string
	patchURL string
	desc     string
	project  string
	variants []string
	tasks    []string

	suite.Suite
}

func TestPatchIntentUnitsSuite(t *testing.T) {
	suite.Run(t, new(PatchIntentUnitsSuite))
}

func (s *PatchIntentUnitsSuite) SetupTest() {
	s.sender = send.MakeInternalLogger()
	s.env = &mock.Environment{}

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.Require().NoError(s.env.Configure(ctx, filepath.Join(evergreen.FindEvergreenHome(), testutil.TestDir, testutil.TestSettings)))
	testutil.ConfigureIntegrationTest(s.T(), s.env.Settings(), "TestPatchIntentUnitsSuite")

	s.NotNil(s.env.Settings())

	s.NoError(db.ClearCollections(model.ProjectVarsCollection, version.Collection, user.Collection, model.ProjectRefCollection, patch.Collection, patch.IntentCollection))
	s.NoError(db.ClearGridCollections(patch.GridFSPrefix))

	s.NoError((&model.ProjectRef{
		Owner:      "evergreen-ci",
		Repo:       "evergreen",
		Identifier: "mci",
		Enabled:    true,
		Branch:     "master",
		RemotePath: "self-tests.yml",
		RepoKind:   "github",
	}).Insert())

	s.NoError((&user.DBUser{
		Id: "github_patch_user",
	}).Insert())

	s.NoError((&model.ProjectVars{
		Id: "mci",
		PatchDefinitions: []model.PatchDefinition{
			{
				Alias:   patch.GithubAlias,
				Variant: "ubuntu.*",
				Task:    "dist.*",
			},
			{
				Alias:   patch.GithubAlias,
				Variant: "race.*",
				Task:    "dist.*",
			},
		},
	}).Insert())

	s.repo = "evergreen-ci/evergreen"
	s.headRepo = "tychoish/evergreen"
	s.prNumber = 448
	s.user = "github_patch_user"
	s.hash = "776f608b5b12cd27b8d931c8ee4ca0c13f857299"
	s.patchURL = "https://github.com/evergreen-ci/evergreen/pull/448.diff"
	s.desc = "Test!"
	s.project = "mci"
	s.variants = []string{"ubuntu1604", "ubuntu1604-arm64", "ubuntu1604-debug", "race-detector"}
	s.tasks = []string{"dist", "dist-test"}

	factory, err := registry.GetJobFactory(patchIntentJobName)
	s.NoError(err)
	s.NotNil(factory)
	s.NotNil(factory())
	s.Equal(factory().Type().Name, patchIntentJobName)
}
func (s *PatchIntentUnitsSuite) TearDownTest() {
	s.cancel()
	evergreen.ResetEnvironment()
}

func (s *PatchIntentUnitsSuite) verifyJob(intent patch.Intent) *patchIntentProcessor {
	j := makePatchIntentProcessor()
	j.Intent = intent
	j.env = s.env
	j.PatchID = bson.NewObjectId()
	j.logger = logging.MakeGrip(s.sender)
	s.False(j.Status().Completed)
	s.NotPanics(func() { j.Run() })
	s.True(j.Status().Completed)
	s.Require().NoError(j.Error())
	s.Require().False(j.HasErrors())

	return j
}

func (s *PatchIntentUnitsSuite) TestProcessCliPatchIntent() {
	patchContent, err := fetchPatchByURL(s.patchURL)
	s.NoError(err)
	s.NotEmpty(patchContent)

	intent, err := patch.NewCliIntent(s.user, s.project, s.hash, "", patchContent, s.desc, true, s.variants, s.tasks, "")
	s.NoError(err)
	s.Require().NotNil(intent)
	s.NoError(intent.Insert())

	j := s.verifyJob(intent)

	patchDoc, err := patch.FindOne(patch.ById(j.PatchID))
	s.NoError(err)
	s.Require().NotNil(patchDoc)

	s.verifyPatchDoc(patchDoc, j.PatchID)
	for _, variant := range patchDoc.BuildVariants {
		s.Contains(s.variants, variant)
	}
	s.Equal(s.tasks, patchDoc.Tasks)

	s.Zero(patchDoc.GithubPatchData)

	s.verifyVersionDoc(patchDoc, evergreen.PatchVersionRequester, j.user.Email())

	s.gridFSFileExists(patchDoc.Patches[0].PatchSet.PatchFileId)
}

func (s *PatchIntentUnitsSuite) TestProcessGithubPatchIntent() {
	intent, err := patch.NewGithubIntent("1", s.prNumber, s.repo, "tychoish/evergreen", "EVG-2387-use-plain-logger", s.hash, "tychoish", s.patchURL)
	s.NoError(err)
	s.NotNil(intent)
	s.NoError(intent.Insert())

	j := s.verifyJob(intent)
	patchDoc, err := patch.FindOne(patch.ById(j.PatchID))
	s.Require().NoError(err)
	s.Require().NotNil(patchDoc)

	s.verifyPatchDoc(patchDoc, j.PatchID)
	s.Len(patchDoc.BuildVariants, 4)
	s.Contains(patchDoc.BuildVariants, "ubuntu1604")
	s.Contains(patchDoc.BuildVariants, "ubuntu1604-arm64")
	s.Contains(patchDoc.BuildVariants, "ubuntu1604-debug")
	s.Contains(patchDoc.BuildVariants, "race-detector")
	s.Len(patchDoc.Tasks, 2)
	s.Contains(patchDoc.Tasks, "dist")
	s.Contains(patchDoc.Tasks, "dist-test")

	s.Equal(s.prNumber, patchDoc.GithubPatchData.PRNumber)
	s.Equal("tychoish", patchDoc.GithubPatchData.Author)
	s.Equal(s.user, patchDoc.Author)
	s.Equal(s.patchURL, patchDoc.GithubPatchData.DiffURL)
	repo := strings.Split(s.repo, "/")
	s.Equal(repo[0], patchDoc.GithubPatchData.BaseOwner)
	s.Equal(repo[1], patchDoc.GithubPatchData.BaseRepo)
	headRepo := strings.Split(s.headRepo, "/")
	s.Equal(headRepo[0], patchDoc.GithubPatchData.HeadOwner)
	s.Equal(headRepo[1], patchDoc.GithubPatchData.HeadRepo)
	s.Equal("776f608b5b12cd27b8d931c8ee4ca0c13f857299", patchDoc.Githash)

	s.verifyVersionDoc(patchDoc, evergreen.GithubPRRequester, j.user.Email())

	s.gridFSFileExists(patchDoc.Patches[0].PatchSet.PatchFileId)
}

func (s *PatchIntentUnitsSuite) verifyPatchDoc(patchDoc *patch.Patch, expectedPatchID bson.ObjectId) {
	s.Equal(evergreen.PatchCreated, patchDoc.Status)
	s.Equal(expectedPatchID, patchDoc.Id)
	s.NotEmpty(patchDoc.Patches)
	s.True(patchDoc.Activated)
	s.NotEmpty(patchDoc.PatchedConfig)
	s.NotZero(patchDoc.CreateTime)
	s.Zero(patchDoc.StartTime)
	s.Zero(patchDoc.FinishTime)
	s.NotEqual(0, patchDoc.PatchNumber)
	s.Equal(s.hash, patchDoc.Githash)

	s.Len(patchDoc.Patches, 1)
	s.Empty(patchDoc.Patches[0].ModuleName)
	s.Equal(s.hash, patchDoc.Patches[0].Githash)
	s.Empty(patchDoc.Patches[0].PatchSet.Patch)
	s.NotEmpty(patchDoc.Patches[0].PatchSet.PatchFileId)
	s.Len(patchDoc.Patches[0].PatchSet.Summary, 2)
}

func (s *PatchIntentUnitsSuite) verifyVersionDoc(patchDoc *patch.Patch, expectedRequester, expectedEmail string) {
	versionDoc, err := version.FindOne(version.ById(patchDoc.Id.Hex()))
	s.NoError(err)
	s.Require().NotNil(versionDoc)

	s.NotZero(versionDoc.CreateTime)
	s.Zero(versionDoc.StartTime)
	s.Zero(versionDoc.FinishTime)
	s.Equal(s.hash, versionDoc.Revision)
	s.Equal(expectedEmail, versionDoc.AuthorEmail)
	s.Equal(patchDoc.Description, versionDoc.Message)
	s.NotZero(versionDoc.Config)
	s.Equal(s.user, versionDoc.Author)
	s.Len(versionDoc.BuildIds, 4)

	s.Require().Len(versionDoc.BuildVariants, 4)
	s.True(versionDoc.BuildVariants[0].Activated)
	s.Zero(versionDoc.BuildVariants[0].ActivateAt)
	s.NotEmpty(versionDoc.BuildVariants[0].BuildId)
	s.Contains(versionDoc.BuildIds, versionDoc.BuildVariants[0].BuildId)

	s.True(versionDoc.BuildVariants[1].Activated)
	s.Zero(versionDoc.BuildVariants[1].ActivateAt)
	s.NotEmpty(versionDoc.BuildVariants[1].BuildId)
	s.Contains(versionDoc.BuildIds, versionDoc.BuildVariants[1].BuildId)

	s.True(versionDoc.BuildVariants[2].Activated)
	s.Zero(versionDoc.BuildVariants[2].ActivateAt)
	s.NotEmpty(versionDoc.BuildVariants[2].BuildId)
	s.Contains(versionDoc.BuildIds, versionDoc.BuildVariants[2].BuildId)

	s.True(versionDoc.BuildVariants[3].Activated)
	s.Zero(versionDoc.BuildVariants[3].ActivateAt)
	s.NotEmpty(versionDoc.BuildVariants[3].BuildId)
	s.Contains(versionDoc.BuildIds, versionDoc.BuildVariants[3].BuildId)

	s.Equal(expectedRequester, versionDoc.Requester)
	s.Empty(versionDoc.Errors)
	s.Empty(versionDoc.Warnings)
	s.Empty(versionDoc.Owner)
	s.Empty(versionDoc.Repo)
	s.Equal(versionDoc.Id, patchDoc.Version)
}

func (s *PatchIntentUnitsSuite) gridFSFileExists(patchFileID string) {
	reader, err := db.GetGridFile(patch.GridFSPrefix, patchFileID)
	s.Require().NoError(err)
	s.Require().NotNil(reader)
	defer reader.Close()
	bytes, err := ioutil.ReadAll(reader)
	s.NoError(err)
	s.NotEmpty(bytes)
}
