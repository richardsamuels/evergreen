package trigger

import (
	"net/http"
	"testing"

	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/stretchr/testify/suite"
)

type payloadSuite struct {
	url    string
	status string
	t      commonTemplateData

	suite.Suite
}

func TestPayloads(t *testing.T) {
	suite.Run(t, &payloadSuite{})
}
func (s *payloadSuite) SetupTest() {
	s.url = "https://example.com/patch/1234"
	s.status = "failed"

	headers := http.Header{
		"X-Evergreen-test": []string{"something"},
	}

	s.t = commonTemplateData{
		ID:              "1234",
		Object:          "patch",
		Project:         "test",
		URL:             s.url,
		PastTenseStatus: s.status,
		Headers:         headers,
	}
}

func (s *payloadSuite) TestEmail() {
	m, err := emailPayload(s.t)
	s.NoError(err)
	s.Require().NotNil(m)

	s.Equal(m.Subject, "Evergreen patch has failed!")
	s.Contains(m.Body, "Your Evergreen patch <")
	s.Contains(m.Body, "> has failed.")
	s.Contains(m.Body, `href="`+s.url+`"`)
	s.Contains(m.Body, "X-Evergreen-test:something")
}

func (s *payloadSuite) TestEvergreenWebhook() {
	model := restModel.APIPatch{}
	model.Author = restModel.ToAPIString("somebody")

	m, err := webhookPayload(&model, s.t.Headers)
	s.NoError(err)
	s.Require().NotNil(m)

	s.Len(m.Body, 410)
	s.Len(m.Headers, 1)
}

func (s *payloadSuite) TestJIRAComment() {
	m, err := jiraComment(s.t)
	s.NoError(err)
	s.Require().NotNil(m)

	s.Equal("Evergreen patch [1234|https://example.com/patch/1234] in 'test' has failed!", *m)
}

func (s *payloadSuite) TestJIRAIssue() {
	m, err := jiraIssue(s.t)
	s.NoError(err)
	s.Require().NotNil(m)

	s.Equal("Evergreen patch '1234' in 'test' has failed", m.Summary)
	s.Equal("Evergreen patch [1234|https://example.com/patch/1234] in 'test' has failed!", m.Description)
}

func (s *payloadSuite) TestSlack() {
	m, err := slack(s.t)
	s.NoError(err)
	s.Require().NotNil(m)

	s.Equal("Evergreen patch <https://example.com/patch/1234|1234> in 'test' has failed!", m.Body)
	s.Empty(m.Attachments)
}