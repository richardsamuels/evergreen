package units

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
)

const (
	eventNotificationJobName = "event-send"

	evergreenWebhookTimeout       = 5 * time.Second
	evergreenNotificationIDHeader = "X-Evergreen-Notification-ID"
	evergreenHMACHeader           = "X-Evergreen-Signature"
)

func init() {
	registry.AddJobType(eventNotificationJobName, func() amboy.Job { return makeEventNotificationJob() })
}

type eventNotificationJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	settings *evergreen.Settings

	NotificationID string `bson:"notification_id" json:"notification_id" yaml:"notification_id"`
}

func makeEventNotificationJob() *eventNotificationJob {
	j := &eventNotificationJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    eventNotificationJobName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())

	return j
}

func newEventNotificationJob(id string) amboy.Job {
	j := makeEventNotificationJob()
	j.NotificationID = id

	j.SetID(fmt.Sprintf("%s:%s", eventNotificationJobName, id))
	return j
}

func (j *eventNotificationJob) Run(_ context.Context) {
	defer j.MarkComplete()

	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		j.AddError(errors.Wrap(err, "error retrieving admin settings"))

	} else if flags == nil {
		j.AddError(errors.Wrap(err, "fetched no service flags configuration"))
	}
	if len(j.NotificationID) == 0 {
		j.AddError(errors.New("notification ID is not valid"))
	}
	if j.settings == nil {
		j.settings, err = evergreen.GetConfig()
		j.AddError(err)
		if err == nil && j.settings == nil {
			j.AddError(errors.New("settings object is nil"))
		}
	}
	if j.HasErrors() {
		return
	}

	n, err := notification.Find(j.NotificationID)
	j.AddError(err)
	if err == nil && n == nil {
		j.AddError(errors.Errorf("can't find notification with ID: '%s", j.NotificationID))
	}
	if j.HasErrors() {
		return
	}

	var sendError error
	switch n.Subscriber.Type {
	case event.GithubPullRequestSubscriberType:
		if err = checkFlag(flags.GithubStatusAPIDisabled); err != nil {
			j.AddError(err)
			j.AddError(n.MarkError(err))
			return
		}

		sendError = j.githubStatus(n)

	case event.SlackSubscriberType:
		if err = checkFlag(flags.SlackNotificationsDisabled); err != nil {
			j.AddError(err)
			j.AddError(n.MarkError(err))
			return
		}
		sendError = j.slackMessage(n)

	case event.JIRAIssueSubscriberType:
		if err = checkFlag(flags.JIRANotificationsDisabled); err != nil {
			j.AddError(err)
			j.AddError(n.MarkError(err))
			return
		}
		sendError = j.jiraIssue(n)

	case event.JIRACommentSubscriberType:
		if err = checkFlag(flags.JIRANotificationsDisabled); err != nil {
			j.AddError(err)
			j.AddError(n.MarkError(err))
			return
		}
		sendError = j.jiraComment(n)

	case event.EvergreenWebhookSubscriberType:
		if err = checkFlag(flags.WebhookNotificationsDisabled); err != nil {
			j.AddError(err)
			j.AddError(n.MarkError(err))
			return
		}
		sendError = j.evergreenWebhook(n)

	case event.EmailSubscriberType:
		if err = checkFlag(flags.EmailNotificationsDisabled); err != nil {
			j.AddError(err)
			j.AddError(n.MarkError(err))
			return
		}
		sendError = j.email(n)

	default:
		j.AddError(errors.Errorf("unknown subscriber type: %s", n.Subscriber.Type))
	}

	j.AddError(n.MarkSent())
	j.AddError(sendError)
	j.AddError(n.MarkError(sendError))
}

func (j *eventNotificationJob) githubStatus(n *notification.Notification) error {
	token, err := j.settings.GetGithubOauthToken()
	if err != nil {
		return errors.New("github-pull-request no auth token")
	}

	subscriber, ok := n.Subscriber.Target.(*event.GithubPullRequestSubscriber)
	if !ok {
		return errors.New("github-pull-request payload is invalid")
	}

	c, err := n.Composer()
	if err != nil {
		return errors.Wrap(err, "github-pull-request error building message")
	}

	sender, err := send.NewGithubStatusLogger("evergreen", &send.GithubOptions{
		Token:   token,
		Account: subscriber.Owner,
		Repo:    subscriber.Repo,
	}, subscriber.Ref)
	if err != nil {
		return errors.New("github-pull-request error building sender")
	}

	j.send(sender, c, n)

	return nil
}

func jiraOptions(c evergreen.JiraConfig) (*send.JiraOptions, error) {
	url, err := url.Parse(c.Host)
	if err != nil {
		return nil, errors.Wrap(err, "invalid JIRA host")
	}
	url.Scheme = "https"

	jiraOpts := send.JiraOptions{
		Name:     "evergreen",
		BaseURL:  url.String(),
		Username: c.Username,
		Password: c.Password,
	}

	return &jiraOpts, nil
}

func (j *eventNotificationJob) jiraComment(n *notification.Notification) error {
	jiraOpts, err := jiraOptions(j.settings.Jira)
	if err != nil {
		return errors.Wrap(err, "jira-comment error building jira settings")
	}

	jiraIssue, ok := n.Subscriber.Target.(*string)
	if !ok {
		return fmt.Errorf("jira-comment subscriber was invalid (expected string)")
	}

	sender, err := send.MakeJiraCommentLogger(*jiraIssue, jiraOpts)
	if err != nil {
		return errors.Wrap(err, "jira-comment sender error")
	}

	c, err := n.Composer()
	if err != nil {
		return errors.Wrap(err, "jira-comment error building message")
	}

	j.send(sender, c, n)

	return nil
}

func (j *eventNotificationJob) jiraIssue(n *notification.Notification) error {
	c, err := n.Composer()
	if err != nil {
		return errors.Wrap(err, "jira-issue error building message")
	}

	jiraOpts, err := jiraOptions(j.settings.Jira)
	if err != nil {
		return errors.Wrap(err, "jira-issue error building jira settings")
	}

	sender, err := send.MakeJiraLogger(jiraOpts)
	if err != nil {
		return errors.Wrap(err, "jira-issue sender error")
	}

	j.send(sender, c, n)

	return nil
}

func (j *eventNotificationJob) evergreenWebhook(n *notification.Notification) error {
	c, err := n.Composer()
	if err != nil {
		return err
	}

	hookSubscriber, ok := n.Subscriber.Target.(*event.WebhookSubscriber)
	if !ok || hookSubscriber == nil {
		return fmt.Errorf("evergreen-webhook invalid subscriber")
	}

	u, err := url.Parse(hookSubscriber.URL)
	if err != nil {
		return errors.Wrap(err, "evergreen-webhook bad URL")
	}

	if !strings.HasPrefix(u.Host, "127.0.0.1:") {
		u.Scheme = "http"
	}

	payload := []byte(c.String())
	reader := bytes.NewReader(payload)
	req, err := http.NewRequest(http.MethodPost, u.String(), reader)
	if err != nil {
		return errors.Wrap(err, "failed to create http request")
	}

	hash, err := util.CalculateHMACHash(hookSubscriber.Secret, payload)
	if err != nil {
		return errors.Wrap(err, "failed to calculate hash")
	}

	req.Header.Del(evergreenHMACHeader)
	req.Header.Add(evergreenHMACHeader, hash)
	req.Header.Del(evergreenNotificationIDHeader)
	req.Header.Add(evergreenNotificationIDHeader, j.NotificationID)

	ctx, cancel := context.WithTimeout(req.Context(), evergreenWebhookTimeout)
	defer cancel()

	req = req.WithContext(ctx)

	client := util.GetHTTPClient()
	defer util.PutHTTPClient(client)

	resp, err := client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return errors.Wrap(err, "evergreen-webhook failed to send webhook data")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.Errorf("evergreen-webhook response status was %d", resp.StatusCode)
	}

	return nil
}

func (j *eventNotificationJob) slackMessage(n *notification.Notification) error {
	// TODO EVG-3086 slack rate limiting
	target, ok := n.Subscriber.Target.(*string)
	if !ok {
		return fmt.Errorf("slack subscriber was invalid (expected string)")
	}

	c, err := n.Composer()
	if err != nil {
		return errors.Wrap(err, "slack error building message")
	}

	opts := send.SlackOptions{
		Channel:       *target,
		Fields:        true,
		AllFields:     true,
		BasicMetadata: false,
		Name:          "evergreen",
	}
	// TODO other attributes

	sender, err := send.NewSlackLogger(&opts, j.settings.Slack.Token, send.LevelInfo{
		Default:   level.Notice,
		Threshold: level.Notice,
	})
	if err != nil {
		return errors.Wrap(err, "slack sender error")
	}

	j.send(sender, c, n)

	return nil
}

func (j *eventNotificationJob) email(n *notification.Notification) error {
	// TODO after MAKE-383 expand this to send custom evergreen headers
	smtpConf := j.settings.Notify.SMTP
	if smtpConf == nil {
		return fmt.Errorf("email smtp settings are empty")
	}
	recipient, ok := n.Subscriber.Target.(*string)
	if !ok {
		return fmt.Errorf("email recipient email is not a string")
	}
	c, err := n.Composer()
	if err != nil {
		return errors.Wrap(err, "email error building message")
	}

	fields, ok := c.Raw().(message.Fields)
	if !ok {
		return fmt.Errorf("email payload is invalid")
	}
	opts := send.SMTPOptions{
		Name:              "evergreen",
		From:              smtpConf.From,
		Server:            smtpConf.Server,
		Port:              smtpConf.Port,
		UseSSL:            smtpConf.UseSSL,
		Username:          smtpConf.Username,
		Password:          smtpConf.Password,
		PlainTextContents: false,
		NameAsSubject:     true,
		GetContents: func(opts *send.SMTPOptions, m message.Composer) (string, string) {
			return fields["subject"].(string), fields["body"].(string)
		},
	}
	if err = opts.AddRecipients(*recipient); err != nil {
		return errors.Wrap(err, "email was invalid")
	}
	sender, err := send.MakeSMTPLogger(&opts)
	if err != nil {
		return errors.Wrap(err, "email settings are invalid")
	}

	j.send(sender, c, n)
	return nil
}

func (j *eventNotificationJob) send(s send.Sender, c message.Composer, n *notification.Notification) {
	err := s.SetErrorHandler(getSendErrorHandler(n))
	grip.Error(message.WrapError(err, message.Fields{
		"message":         "failed to set error handler",
		"notification_id": n.ID,
	}))
	s.Send(c)
}

func getSendErrorHandler(n *notification.Notification) send.ErrorHandler {
	return func(err error, c message.Composer) {
		if err == nil || c == nil {
			return
		}

		err = n.MarkError(err)
		grip.Error(message.WrapError(err, message.Fields{
			"job":             eventMetaJobName,
			"notification_id": n.ID,
			"source":          "events-processing",
			"message":         "failed to add error to notification",
			"composer":        c.String(),
		}))
	}
}

func checkFlag(flag bool) error {
	if flag {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"job":     eventNotificationJobName,
			"message": "sender is disabled, not sending notification",
		})
		return errors.New("sender is disabled, not sending notification")
	}

	return nil
}
