package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
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
	env      evergreen.Environment
	flags    *evergreen.ServiceFlags

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

func (j *eventNotificationJob) setup() error {
	if len(j.NotificationID) == 0 {
		return errors.New("notification ID is not valid")
	}

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}
	if j.flags == nil {
		j.flags = &evergreen.ServiceFlags{}
		if err := j.flags.Get(); err != nil {
			return errors.Wrap(err, "error retrieving service flags")

		} else if j.flags == nil {
			return errors.Wrap(err, "fetched no service flags configuration")
		}
	}

	return nil
}

func (j *eventNotificationJob) Run(_ context.Context) {
	defer j.MarkComplete()

	if err := j.setup(); err != nil {
		j.AddError(err)
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

	if err = j.checkDegradedMode(n); err != nil {
		j.AddError(err)
		j.AddError(n.MarkError(err))
		return
	}

	err = j.send(n)
	grip.Error(message.WrapError(err, message.Fields{
		"job_id":          j.ID(),
		"notification_id": n.ID,
		"message":         "send failed",
	}))
	j.AddError(err)
	j.AddError(n.MarkSent())
	j.AddError(n.MarkError(err))
}

func (j *eventNotificationJob) send(n *notification.Notification) error {
	c, err := n.Composer()
	if err != nil {
		return err
	}

	key, err := n.SenderKey()
	if err != nil {
		return errors.Wrap(err, "can't build sender for notification")
	}

	sender, err := j.env.GetSender(key)
	if err != nil {
		return errors.Wrap(err, "error building sender for notification")
	}
	if err = sender.SetLevel(send.LevelInfo{
		Default:   level.Notice,
		Threshold: level.Notice,
	}); err != nil {
		return errors.Wrap(err, "error setting level in sender")
	}

	err = sender.SetErrorHandler(getSendErrorHandler(n))
	grip.Error(message.WrapError(err, message.Fields{
		"message":         "failed to set error handler",
		"notification_id": n.ID,
	}))
	sender.Send(c)

	return nil
}

func (j *eventNotificationJob) checkDegradedMode(n *notification.Notification) error {
	switch n.Subscriber.Type {
	case event.GithubPullRequestSubscriberType:
		return checkFlag(j.flags.GithubStatusAPIDisabled)

	case event.SlackSubscriberType:
		return checkFlag(j.flags.SlackNotificationsDisabled)

	case event.JIRAIssueSubscriberType:
		return checkFlag(j.flags.JIRANotificationsDisabled)

	case event.JIRACommentSubscriberType:
		return checkFlag(j.flags.JIRANotificationsDisabled)

	case event.EvergreenWebhookSubscriberType:
		return checkFlag(j.flags.WebhookNotificationsDisabled)

	case event.EmailSubscriberType:
		return checkFlag(j.flags.EmailNotificationsDisabled)

	default:
		return errors.Errorf("unknown subscriber type: %s", n.Subscriber.Type)
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
